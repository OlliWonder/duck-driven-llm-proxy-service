package store

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"
)

func testKey() []byte {
	return bytes.Repeat([]byte{0x42}, 32)
}

func TestPutGet(t *testing.T) {
	s, err := New(testKey(), time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}
	rec := Record{Original: []byte("orig"), Masked: []byte("mask")}
	s.Put("id1", rec)
	got, err := s.Get("id1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got.Original) != "orig" || string(got.Masked) != "mask" {
		t.Fatalf("unexpected record: %+v", got)
	}
}

func TestGetNotFound(t *testing.T) {
	s, _ := New(testKey(), time.Hour, 10)
	_, err := s.Get("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTTLExpiry(t *testing.T) {
	now := time.Now()
	s, _ := New(testKey(), time.Hour, 10)
	s.now = func() time.Time { return now }
	s.Put("id1", Record{Original: []byte("o"), Masked: []byte("m")})

	// Сдвигаем время за пределы TTL.
	s.now = func() time.Time { return now.Add(2 * time.Hour) }
	_, err := s.Get("id1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected expiry, got %v", err)
	}
}

func TestEvictionOnOverflow(t *testing.T) {
	s, _ := New(testKey(), time.Hour, 2)
	s.Put("a", Record{Original: []byte("1"), Masked: []byte("1")})
	s.Put("b", Record{Original: []byte("2"), Masked: []byte("2")})
	// Касаемся "a", чтобы "b" стала LRU.
	_, _ = s.Get("a")
	s.Put("c", Record{Original: []byte("3"), Masked: []byte("3")})
	if s.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", s.Len())
	}
	if _, err := s.Get("b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected b evicted, got %v", err)
	}
	if _, err := s.Get("a"); err != nil {
		t.Fatalf("a should remain: %v", err)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	s, _ := New(testKey(), time.Hour, 10)
	ct, err := s.Encrypt([]byte("секрет"))
	if err != nil {
		t.Fatal(err)
	}
	if string(ct) == "секрет" {
		t.Fatal("ciphertext must not equal plaintext")
	}
	pt, err := s.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "секрет" {
		t.Fatalf("decrypt mismatch: %q", pt)
	}
}

func TestConcurrentSameID(t *testing.T) {
	s, _ := New(testKey(), time.Hour, 100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Put("id", Record{Original: []byte("o"), Masked: []byte("m")})
			_, _ = s.Get("id")
		}()
	}
	wg.Wait()
	if s.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", s.Len())
	}
}

func TestBadKey(t *testing.T) {
	if _, err := New([]byte("short"), time.Hour, 10); err == nil {
		t.Fatal("expected error for short key")
	}
}
