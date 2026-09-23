package api

import "sync"

// keyedMutex обеспечивает взаимное исключение по ключу, чтобы решение о
// маскировании и демаскировании для одного payload_id было атомарным при
// конкурентных запросах.
type keyedMutex struct {
	mu sync.Mutex
	m  map[string]*keyLock
}

type keyLock struct {
	mu   sync.Mutex
	refs int
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{m: make(map[string]*keyLock)}
}

// Lock возвращает мьютекс для ключа и блокирует его.
func (k *keyedMutex) Lock(key string) {
	k.mu.Lock()
	l, ok := k.m[key]
	if !ok {
		l = &keyLock{}
		k.m[key] = l
	}
	l.refs++
	k.mu.Unlock()
	l.mu.Lock()
}

// Unlock разблокирует мьютекс для ключа.
func (k *keyedMutex) Unlock(key string) {
	k.mu.Lock()
	l, ok := k.m[key]
	if !ok {
		k.mu.Unlock()
		panic("api: unlock of unknown keyed mutex")
	}
	l.refs--
	if l.refs == 0 {
		delete(k.m, key)
	}
	l.mu.Unlock()
	k.mu.Unlock()
}
