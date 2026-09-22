package api

import "sync"

// keyedMutex обеспечивает взаимное исключение по ключу, чтобы решение о
// маскировании и демаскировании для одного payload_id было атомарным при
// конкурентных запросах.
type keyedMutex struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{m: make(map[string]*sync.Mutex)}
}

// Lock возвращает мьютекс для ключа и блокирует его.
func (k *keyedMutex) Lock(key string) {
	k.mu.Lock()
	l, ok := k.m[key]
	if !ok {
		l = &sync.Mutex{}
		k.m[key] = l
	}
	k.mu.Unlock()
	l.Lock()
}

// Unlock разблокирует мьютекс для ключа.
func (k *keyedMutex) Unlock(key string) {
	k.mu.Lock()
	l := k.m[key]
	k.mu.Unlock()
	l.Unlock()
}