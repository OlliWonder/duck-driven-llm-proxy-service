package ner

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

func frag(t pii.Type, start, end int) detection.Fragment {
	return detection.Fragment{Type: t, Start: start, End: end}
}

// TestCacheMiss проверяет, что при первом обращении fn вызывается и результат
// возвращается.
func TestCacheMiss(t *testing.T) {
	c := NewCache(CacheOptions{TTL: time.Minute, MaxSize: 10})
	ctx := context.Background()
	var calls int32

	got, err := c.Get(ctx, "клиент Иван Петров", func() ([]detection.Fragment, error) {
		atomic.AddInt32(&calls, 1)
		return []detection.Fragment{frag(pii.TypeFullName, 0, 15)}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	if len(got) != 1 || got[0].Type != pii.TypeFullName {
		t.Fatalf("unexpected result: %+v", got)
	}
	s := c.Stats()
	if s.Misses != 1 || s.Hits != 0 {
		t.Fatalf("expected 1 miss 0 hits, got %+v", s)
	}
}

// TestCacheHit проверяет, что повторное обращение с тем же текстом не вызывает
// fn и возвращает закешированный результат.
func TestCacheHit(t *testing.T) {
	c := NewCache(CacheOptions{TTL: time.Minute, MaxSize: 10})
	ctx := context.Background()
	var calls int32

	fn := func() ([]detection.Fragment, error) {
		atomic.AddInt32(&calls, 1)
		return []detection.Fragment{frag(pii.TypeFullName, 0, 15)}, nil
	}

	if _, err := c.Get(ctx, "клиент Иван Петров", fn); err != nil {
		t.Fatalf("first call error: %v", err)
	}
	got, err := c.Get(ctx, "клиент Иван Петров", fn)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected fn called once, got %d", calls)
	}
	if len(got) != 1 {
		t.Fatalf("unexpected result: %+v", got)
	}
	s := c.Stats()
	if s.Hits != 1 || s.Misses != 1 {
		t.Fatalf("expected 1 hit 1 miss, got %+v", s)
	}
}

// TestCacheTTL проверяет, что после истечения TTL fn вызывается снова.
func TestCacheTTL(t *testing.T) {
	c := NewCache(CacheOptions{TTL: 50 * time.Millisecond, MaxSize: 10})
	ctx := context.Background()
	var calls int32

	fn := func() ([]detection.Fragment, error) {
		atomic.AddInt32(&calls, 1)
		return []detection.Fragment{frag(pii.TypeFullName, 0, 15)}, nil
	}

	if _, err := c.Get(ctx, "клиент Иван Петров", fn); err != nil {
		t.Fatalf("first call error: %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	if _, err := c.Get(ctx, "клиент Иван Петров", fn); err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected fn called twice after TTL, got %d", calls)
	}
}

// TestCacheEviction проверяет, что при превышении MaxSize самые старые записи
// вытесняются и fn вызывается снова для вытесненного ключа.
func TestCacheEviction(t *testing.T) {
	c := NewCache(CacheOptions{TTL: time.Minute, MaxSize: 2})
	ctx := context.Background()
	var calls int32

	fn := func() ([]detection.Fragment, error) {
		atomic.AddInt32(&calls, 1)
		return []detection.Fragment{frag(pii.TypeFullName, 0, 15)}, nil
	}

	// Заполняем кеш до лимита.
	for i := 0; i < 2; i++ {
		if _, err := c.Get(ctx, "текст "+itoa(i), fn); err != nil {
			t.Fatalf("fill call error: %v", err)
		}
	}
	if c.Len() != 2 {
		t.Fatalf("expected len 2, got %d", c.Len())
	}
	// Третий ключ вытесняет самый старый (текст 0).
	if _, err := c.Get(ctx, "текст 2", fn); err != nil {
		t.Fatalf("third call error: %v", err)
	}
	if c.Len() != 2 {
		t.Fatalf("expected len 2 after eviction, got %d", c.Len())
	}
	// Текст 0 вытеснен — fn вызывается снова.
	before := atomic.LoadInt32(&calls)
	if _, err := c.Get(ctx, "текст 0", fn); err != nil {
		t.Fatalf("re-get evicted error: %v", err)
	}
	if atomic.LoadInt32(&calls) != before+1 {
		t.Fatalf("expected fn called again for evicted key, calls=%d before=%d", calls, before)
	}
}

// TestCacheConcurrentSameText проверяет singleflight: при одновременных
// запросах с одинаковым текстом fn вызывается ровно один раз, все получают
// результат.
func TestCacheConcurrentSameText(t *testing.T) {
	c := NewCache(CacheOptions{TTL: time.Minute, MaxSize: 10})
	ctx := context.Background()
	var calls int32

	// fn блокируется, чтобы гарантировать перекрытие запросов.
	release := make(chan struct{})
	fn := func() ([]detection.Fragment, error) {
		atomic.AddInt32(&calls, 1)
		<-release
		return []detection.Fragment{frag(pii.TypeFullName, 0, 15)}, nil
	}

	const n = 32
	var wg sync.WaitGroup
	errs := make([]error, n)
	results := make([][]detection.Fragment, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = c.Get(ctx, "клиент Иван Петров", fn)
		}(i)
	}
	// Даём всем goroutine дойти до singleflight.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected fn called once, got %d", calls)
	}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("call %d error: %v", i, errs[i])
		}
		if len(results[i]) != 1 || results[i][0].Type != pii.TypeFullName {
			t.Fatalf("call %d unexpected result: %+v", i, results[i])
		}
	}
}

// TestCacheConcurrentSameTextNoRepeatNER проверяет, что при повторном
// одновременном обращении к уже закешированному тексту fn не вызывается вовсе.
func TestCacheConcurrentSameTextNoRepeatNER(t *testing.T) {
	c := NewCache(CacheOptions{TTL: time.Minute, MaxSize: 10})
	ctx := context.Background()
	var calls int32

	fn := func() ([]detection.Fragment, error) {
		atomic.AddInt32(&calls, 1)
		return []detection.Fragment{frag(pii.TypeFullName, 0, 15)}, nil
	}

	// Первый вызов заполняет кеш.
	if _, err := c.Get(ctx, "клиент Иван Петров", fn); err != nil {
		t.Fatalf("first call error: %v", err)
	}
	// Множество одновременных повторных вызовов — все должны попасть в кеш.
	const n = 32
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Get(ctx, "клиент Иван Петров", fn); err != nil {
				t.Errorf("repeat call error: %v", err)
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected fn called once total, got %d", calls)
	}
}

// TestCacheReturnsIndependentCopy проверяет, что каждый вызов возвращает
// независимый срез: мутация результата не влияет на кеш.
func TestCacheReturnsIndependentCopy(t *testing.T) {
	c := NewCache(CacheOptions{TTL: time.Minute, MaxSize: 10})
	ctx := context.Background()

	fn := func() ([]detection.Fragment, error) {
		return []detection.Fragment{frag(pii.TypeFullName, 0, 15)}, nil
	}

	first, err := c.Get(ctx, "клиент Иван Петров", fn)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	// Мутируем первый результат.
	first[0].Type = pii.TypeAddress
	first[0].Start = 99

	second, err := c.Get(ctx, "клиент Иван Петров", fn)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if second[0].Type != pii.TypeFullName || second[0].Start != 0 {
		t.Fatalf("cache was mutated: %+v", second)
	}
}

// TestCacheErrorNotCached проверяет, что ошибка fn не кешируется и следующий
// вызов повторяет fn.
func TestCacheErrorNotCached(t *testing.T) {
	c := NewCache(CacheOptions{TTL: time.Minute, MaxSize: 10})
	ctx := context.Background()
	var calls int32

	fn := func() ([]detection.Fragment, error) {
		atomic.AddInt32(&calls, 1)
		return nil, errors.New("ner unavailable")
	}

	if _, err := c.Get(ctx, "клиент Иван Петров", fn); err == nil {
		t.Fatal("expected error")
	}
	if _, err := c.Get(ctx, "клиент Иван Петров", fn); err == nil {
		t.Fatal("expected error on second call")
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected fn called twice (errors not cached), got %d", calls)
	}
}

// TestCacheOversizedResultNotCached проверяет, что результат, превышающий
// MaxResultBytes, не кешируется и fn вызывается снова.
func TestCacheOversizedResultNotCached(t *testing.T) {
	// Лимит в 1 фрагмент: результат из 2 фрагментов не должен кешироваться.
	c := NewCache(CacheOptions{TTL: time.Minute, MaxSize: 10, MaxResultBytes: int(unsafe.Sizeof(detection.Fragment{}))})
	ctx := context.Background()
	var calls int32

	fn := func() ([]detection.Fragment, error) {
		atomic.AddInt32(&calls, 1)
		return []detection.Fragment{
			frag(pii.TypeFullName, 0, 15),
			frag(pii.TypeAddress, 20, 30),
		}, nil
	}

	if _, err := c.Get(ctx, "клиент Иван Петров", fn); err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if _, err := c.Get(ctx, "клиент Иван Петров", fn); err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected fn called twice (oversized result not cached), got %d", calls)
	}
	if c.Len() != 0 {
		t.Fatalf("expected empty cache, got %d entries", c.Len())
	}
}

func TestCacheInflightBoundAndCleanup(t *testing.T) {
	c := NewCache(CacheOptions{TTL: time.Minute, MaxSize: 10, MaxInflight: 2})
	release := make(chan struct{})
	fn := func() ([]detection.Fragment, error) {
		<-release
		return nil, nil
	}

	var wg sync.WaitGroup
	for _, text := range []string{"first", "second"} {
		wg.Add(1)
		go func(text string) {
			defer wg.Done()
			_, _ = c.Get(context.Background(), text, fn)
		}(text)
	}
	deadline := time.Now().Add(time.Second)
	for c.InflightLen() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := c.InflightLen(); got != 2 {
		t.Fatalf("expected 2 inflight entries, got %d", got)
	}
	if _, err := c.Get(context.Background(), "third", fn); !errors.Is(err, ErrTooManyInflight) {
		t.Fatalf("expected ErrTooManyInflight, got %v", err)
	}
	close(release)
	wg.Wait()
	if got := c.InflightLen(); got != 0 {
		t.Fatalf("inflight entries leaked: %d", got)
	}
}

func TestCachePanicDoesNotLeakInflight(t *testing.T) {
	c := NewCache(CacheOptions{MaxInflight: 1})
	if _, err := c.Get(context.Background(), "panic", func() ([]detection.Fragment, error) {
		panic("boom")
	}); err == nil {
		t.Fatal("expected panic to be returned as an error")
	}
	if got := c.InflightLen(); got != 0 {
		t.Fatalf("inflight entry leaked after panic: %d", got)
	}
}
