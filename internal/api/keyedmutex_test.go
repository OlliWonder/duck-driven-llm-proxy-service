package api

import (
	"sync"
	"testing"
)

func TestKeyedMutexReleasesUnusedKeys(t *testing.T) {
	k := newKeyedMutex()
	for i := 0; i < 1000; i++ {
		key := string(rune(i))
		k.Lock(key)
		k.Unlock(key)
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	if len(k.m) != 0 {
		t.Fatalf("unused keyed mutexes retained: %d", len(k.m))
	}
}

func TestKeyedMutexSerializesSameKey(t *testing.T) {
	k := newKeyedMutex()
	var wg sync.WaitGroup
	inside := 0
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			k.Lock("same")
			defer k.Unlock("same")
			inside++
			if inside != 1 {
				t.Errorf("same key entered concurrently: %d", inside)
			}
			inside--
		}()
	}
	wg.Wait()
}
