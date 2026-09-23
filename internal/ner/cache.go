// Bounded cache результатов detection с TTL, жёстким лимитом размера и
// singleflight (coalescing одинаковых одновременных запросов).
//
// Ключ — хеш текста + версии детектора (исходный текст в ключе не хранится).
// Каждый вызов Get возвращает независимую копию результата, чтобы вызывающий
// не мог мутировать общую память кеша.
package ner

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
)

// DetectorVersion — версия логики детектора. Включается в ключ кеша, чтобы при
// изменении правил валидатора/детектора старые результаты не попадали в новые.
const DetectorVersion = "v2"

const defaultMaxInflight = 512

var ErrTooManyInflight = errors.New("ner: too many distinct detection requests in flight")

// CacheOptions — параметры bounded cache результатов detection.
type CacheOptions struct {
	// TTL — время жизни записи.
	TTL time.Duration
	// MaxSize — жёсткий лимит числа записей.
	MaxSize int
	// MaxResultBytes — максимальный размер результата (в байтах), который
	// разрешено кешировать. Результаты больше лимита не кешируются, чтобы не
	// допустить неограниченного роста памяти.
	MaxResultBytes int
	// MaxInflight ограничивает число разных одновременно вычисляемых ключей.
	// Ожидающие уже существующий singleflight-ключ не занимают новые слоты.
	MaxInflight int
}

// cacheEntry — одна запись кеша.
type cacheEntry struct {
	frags     []detection.Fragment
	expiresAt time.Time
	seq       uint64 // порядок вставки для eviction
}

// inflight — один выполняющийся запрос (singleflight).
type inflight struct {
	done  chan struct{}
	frags []detection.Fragment
	err   error
}

// CacheStats — счётчики попаданий/промахов кеша.
type CacheStats struct {
	Hits   int64
	Misses int64
}

// Cache — bounded cache результатов detection.
type Cache struct {
	ttl            time.Duration
	maxSize        int
	maxResultBytes int
	maxInflight    int

	mu       sync.Mutex
	entries  map[[sha256.Size]byte]*cacheEntry
	inflight map[[sha256.Size]byte]*inflight
	seq      uint64
	hits     int64
	misses   int64
}

// NewCache создаёт кеш с заданными параметрами. Некорректные значения
// заменяются значениями по умолчанию (TTL 60s, MaxSize 10000,
// MaxResultBytes 64KB).
func NewCache(opts CacheOptions) *Cache {
	if opts.TTL <= 0 {
		opts.TTL = 60 * time.Second
	}
	if opts.MaxSize <= 0 {
		opts.MaxSize = 10000
	}
	if opts.MaxResultBytes <= 0 {
		opts.MaxResultBytes = 64 * 1024
	}
	if opts.MaxInflight <= 0 {
		opts.MaxInflight = defaultMaxInflight
	}
	return &Cache{
		ttl:            opts.TTL,
		maxSize:        opts.MaxSize,
		maxResultBytes: opts.MaxResultBytes,
		maxInflight:    opts.MaxInflight,
		entries:        make(map[[sha256.Size]byte]*cacheEntry),
		inflight:       make(map[[sha256.Size]byte]*inflight),
	}
}

// cacheKey возвращает SHA-256 хеш текста и версии детектора. Исходный текст в
// ключе не хранится.
func cacheKey(text string) [sha256.Size]byte {
	h := sha256.New()
	_, _ = h.Write([]byte(DetectorVersion))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(text))
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

// Get возвращает закешированный результат или вычисляет его через fn.
// Одновременные запросы с одинаковым ключом coalesce: только один вызывает fn,
// остальные ждут его результат. Возвращаемый срез — независимая копия.
func (c *Cache) Get(ctx context.Context, text string, fn func() ([]detection.Fragment, error)) ([]detection.Fragment, error) {
	key := cacheKey(text)
	now := time.Now()

	c.mu.Lock()
	// Попадание в кеш.
	if e, ok := c.entries[key]; ok {
		if now.Before(e.expiresAt) {
			c.hits++
			c.mu.Unlock()
			return cloneFrags(e.frags), nil
		}
		// Истёк — удаляем.
		delete(c.entries, key)
	}
	// Уже выполняется такой же запрос — ждём его результат.
	if fl, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		select {
		case <-fl.done:
			return cloneFrags(fl.frags), fl.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if len(c.inflight) >= c.maxInflight {
		c.mu.Unlock()
		return nil, fmt.Errorf("%w: %w", detection.ErrOverloaded, ErrTooManyInflight)
	}
	// Создаём inflight и выполняем fn вне блокировки.
	fl := &inflight{done: make(chan struct{})}
	c.inflight[key] = fl
	c.misses++
	c.mu.Unlock()

	frags, err := invokeDetection(fn)

	c.mu.Lock()
	delete(c.inflight, key)
	if err == nil {
		c.insertLocked(key, frags, now)
	}
	c.mu.Unlock()

	// Записываем результат ДО закрытия канала, чтобы ожидающие увидели его
	// после close (happens-before через закрытие канала).
	fl.frags = frags
	fl.err = err
	close(fl.done)

	return cloneFrags(frags), err
}

func invokeDetection(fn func() ([]detection.Fragment, error)) (frags []detection.Fragment, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			frags = nil
			err = fmt.Errorf("ner: detection panic: %v", recovered)
		}
	}()
	return fn()
}

// insertLocked вставляет запись, если результат не превышает лимит памяти.
// При превышении лимита размера удаляет истёкшие, затем самые старые по
// порядку вставки.
func (c *Cache) insertLocked(key [sha256.Size]byte, frags []detection.Fragment, now time.Time) {
	// Не кешируем результаты, превышающие разумный лимит памяти.
	if resultBytes(frags) > c.maxResultBytes {
		return
	}
	c.seq++
	c.entries[key] = &cacheEntry{
		frags:     frags,
		expiresAt: now.Add(c.ttl),
		seq:       c.seq,
	}
	if len(c.entries) <= c.maxSize {
		return
	}
	// Сначала истёкшие.
	for k, e := range c.entries {
		if !now.Before(e.expiresAt) {
			delete(c.entries, k)
		}
	}
	// Если всё ещё превышен — самые старые по seq.
	for len(c.entries) > c.maxSize {
		var oldestKey [sha256.Size]byte
		var oldestSeq uint64 = ^uint64(0)
		for k, e := range c.entries {
			if e.seq < oldestSeq {
				oldestSeq = e.seq
				oldestKey = k
			}
		}
		delete(c.entries, oldestKey)
	}
}

// resultBytes оценивает размер результата в байтах (срез фрагментов).
func resultBytes(frags []detection.Fragment) int {
	return len(frags) * int(unsafe.Sizeof(detection.Fragment{}))
}

// Stats возвращает счётчики попаданий/промахов.
func (c *Cache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{Hits: c.hits, Misses: c.misses}
}

// ResetStats обнуляет счётчики попаданий/промахов.
func (c *Cache) ResetStats() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits = 0
	c.misses = 0
}

// Len возвращает текущее число записей в кеше.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// InflightLen возвращает число активных различных singleflight-ключей.
func (c *Cache) InflightLen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.inflight)
}

// cloneFrags возвращает независимую копию среза фрагментов.
func cloneFrags(frags []detection.Fragment) []detection.Fragment {
	if frags == nil {
		return nil
	}
	out := make([]detection.Fragment, len(frags))
	copy(out, frags)
	return out
}
