// Пакет store предоставляет in-memory хранилище соответствий, сопоставляющее
// payload_id с его замаскированной и исходной формами.
//
// Гарантии:
//   - Атомарность по payload_id: конкурентные запросы для одного id
//     сериализуются, поэтому пара «маскирование/демаскирование» никогда не
//     перемежается.
//   - Идемпотентность: повтор оригинала возвращает ту же маску; повтор
//     маски возвращает оригинал.
//   - TTL и лимит размера ограничивают память. При переполнении удаляется
//     самая старая запись; активные (недавно использованные) записи никогда
//     не удаляются молча.
//   - Сохранённые значения шифруются в памяти (AES-GCM).
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"sync"
	"time"
)

// ErrNotFound возвращается, когда для payload_id нет записи.
var ErrNotFound = errors.New("store: no record for payload_id")

// Record хранит зашифрованные исходную и замаскированную формы для одного payload_id.
type Record struct {
	Original []byte // зашифрованный исходный текст
	Masked   []byte // зашифрованный замаскированный текст
	Mappings []byte // зашифрованные соответствия токенов (для восстановления)
	Created  time.Time
	LastUsed time.Time
}

// Store — потокобезопасное in-memory хранилище соответствий.
type Store struct {
	mu       sync.Mutex
	entries  map[string]*entry
	key      []byte
	ttl      time.Duration
	maxSize  int
	now      func() time.Time
	evictCb  func(id string)
}

type entry struct {
	rec      Record
	lock     sync.Mutex // сериализация по id
}

// New создаёт хранилище с заданным ключом AES (32 байта для AES-256),
// TTL и максимальным числом записей.
func New(key []byte, ttl time.Duration, maxSize int) (*Store, error) {
	if len(key) != 32 {
		return nil, errors.New("store: AES key must be 32 bytes")
	}
	if ttl <= 0 {
		return nil, errors.New("store: ttl must be positive")
	}
	if maxSize <= 0 {
		return nil, errors.New("store: maxSize must be positive")
	}
	return &Store{
		entries: make(map[string]*entry),
		key:     key,
		ttl:     ttl,
		maxSize: maxSize,
		now:     time.Now,
	}, nil
}

// SetEvictCallback регистрирует обратный вызов, вызываемый с id удалённой
// записи (используется для метрик/логирования). Может быть nil.
func (s *Store) SetEvictCallback(cb func(id string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictCb = cb
}

// Get возвращает запись для id или ErrNotFound.
func (s *Store) Get(id string) (Record, error) {
	s.mu.Lock()
	e, ok := s.entries[id]
	s.mu.Unlock()
	if !ok {
		return Record{}, ErrNotFound
	}
	e.lock.Lock()
	defer e.lock.Unlock()
	if s.expired(e) {
		s.removeLocked(id)
		return Record{}, ErrNotFound
	}
	e.rec.LastUsed = s.now()
	return e.rec, nil
}

// Put сохраняет запись для id, удаляя самую старую запись при достижении лимита.
func (s *Store) Put(id string, rec Record) {
	s.mu.Lock()
	e, ok := s.entries[id]
	if !ok {
		e = &entry{}
		s.entries[id] = e
	}
	s.mu.Unlock()

	e.lock.Lock()
	defer e.lock.Unlock()
	e.rec = rec
	e.rec.LastUsed = s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictIfNeededLocked()
}

// Remove удаляет запись для id.
func (s *Store) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked(id)
}

// Len возвращает текущее число записей.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *Store) expired(e *entry) bool {
	return s.now().Sub(e.rec.LastUsed) > s.ttl
}

func (s *Store) removeLocked(id string) {
	if _, ok := s.entries[id]; ok {
		delete(s.entries, id)
		if s.evictCb != nil {
			s.evictCb(id)
		}
	}
}

// evictIfNeededLocked удаляет наименее недавно использованную запись, когда
// хранилище достигло лимита. Она никогда не удаляет только что записанную запись.
func (s *Store) evictIfNeededLocked() {
	if len(s.entries) <= s.maxSize {
		return
	}
	var oldestID string
	var oldest time.Time
	for id, e := range s.entries {
		if oldestID == "" || e.rec.LastUsed.Before(oldest) {
			oldestID = id
			oldest = e.rec.LastUsed
		}
	}
	if oldestID != "" {
		s.removeLocked(oldestID)
	}
}

// Encrypt запечатывает открытый текст с помощью AES-GCM на ключе хранилища.
func (s *Store) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt открывает шифротекст, запечатанный Encrypt.
func (s *Store) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, errors.New("store: ciphertext too short")
	}
	nonce, ct := ciphertext[:ns], ciphertext[ns:]
	return gcm.Open(nil, nonce, ct, nil)
}