package ner

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestClient создаёт клиент с маленьким maxWait для быстрых тестов.
func newTestClient(t *testing.T) *Client {
	t.Helper()
	skipIfUnavailable(t)
	return NewClientWithOptions(testBaseURL, Options{
		MaxBatch:  16,
		MaxWait:   2 * time.Millisecond,
		QueueSize: 256,
	})
}

// TestMicrobatchOrder проверяет, что результаты распределяются по своим
// элементам в правильном порядке при конкурентных Detect.
func TestMicrobatchOrder(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()
	ctx := context.Background()

	texts := []string{
		"клиент Иван Петров 1",
		"клиент Пётр Сидоров 2",
		"клиент Алексей Кузнецов 3",
		"клиент Дмитрий Смирнов 4",
		"клиент Николай Волков 5",
	}

	results := make([][]Candidate, len(texts))
	var wg sync.WaitGroup
	for i, text := range texts {
		wg.Add(1)
		go func(i int, text string) {
			defer wg.Done()
			cands, err := c.Detect(ctx, text)
			if err != nil {
				t.Errorf("Detect(%q) error: %v", text, err)
				return
			}
			results[i] = cands
		}(i, text)
	}
	wg.Wait()

	// Каждый результат должен соответствовать своему тексту.
	for i, text := range texts {
		if len(results[i]) == 0 {
			t.Fatalf("text %q: no candidates", text)
		}
		// Хотя бы один кандидат должен быть подстрокой своего текста.
		found := false
		for _, cand := range results[i] {
			if strings.Contains(text, cand.Text) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("text %q: candidates %+v do not match input", text, results[i])
		}
	}
}

// TestMicrobatchConcurrent проверяет корректность при большом числе
// конкурентных Detect.
func TestMicrobatchConcurrent(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()
	ctx := context.Background()

	const n = 64
	texts := make([]string, n)
	for i := 0; i < n; i++ {
		texts[i] = "клиент Иван Петров " + itoa(i)
	}

	results := make([][]Candidate, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = c.Detect(ctx, texts[i])
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("Detect(%q) error: %v", texts[i], errs[i])
		}
		if len(results[i]) == 0 {
			t.Fatalf("text %q: no candidates", texts[i])
		}
		found := false
		for _, cand := range results[i] {
			if strings.Contains(texts[i], cand.Text) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("text %q: candidates %+v do not match", texts[i], results[i])
		}
	}
}

// TestMicrobatchContextCancellation проверяет, что отменённый контекст
// возвращает ошибку и не зависает.
func TestMicrobatchContextCancellation(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // отменяем до вызова

	_, err := c.Detect(ctx, "клиент Иван Петров")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestMicrobatchNoGoroutineLeak проверяет, что после Close воркер завершается
// и goroutine не утекают. Допускается одна дополнительная goroutine — фоновый
// reaper http.Transport (IdleConnTimeout), который живёт столько же, сколько
// сам Transport, и не является утечкой.
func TestMicrobatchNoGoroutineLeak(t *testing.T) {
	skipIfUnavailable(t)

	before := runtime.NumGoroutine()

	c := NewClientWithOptions(testBaseURL, Options{
		MaxBatch:  16,
		MaxWait:   2 * time.Millisecond,
		QueueSize: 256,
	})
	ctx := context.Background()
	for i := 0; i < 32; i++ {
		_, _ = c.Detect(ctx, "клиент Иван Петров "+itoa(i))
	}
	c.Close()
	// Закрываем простаивающие keep-alive соединения, чтобы их goroutine
	// (readLoop/writeLoop) завершились и не считались утечкой.
	c.CloseIdleConnections()

	// Даём время на завершение.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before+1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("goroutine leak: before=%d after=%d", before, runtime.NumGoroutine())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestClientDiagnosticStatsAreBounded(t *testing.T) {
	c := &Client{}
	for i := 0; i < maxStatsSamples*3; i++ {
		c.recordBatch(i)
		c.recordQueueWait(time.Duration(i))
		c.recordHTTPRTT(time.Duration(i))
		c.recordSidecarTiming(time.Duration(i), time.Duration(i))
	}
	stats := c.Stats()
	for name, size := range map[string]int{
		"batch": len(stats.BatchSizes), "queue": len(stats.QueueWaits), "http": len(stats.HTTPRTTs),
		"sidecar_map": len(stats.SidecarMap), "sidecar_total": len(stats.SidecarTotal),
	} {
		if size > maxStatsSamples {
			t.Fatalf("%s stats grew past bound: %d", name, size)
		}
	}
}

func TestDetectAfterCloseReturnsError(t *testing.T) {
	c := NewClientWithOptions("http://127.0.0.1:1", Options{MaxWait: time.Millisecond})
	c.Close()
	if _, err := c.Detect(context.Background(), "клиент Иван Петров"); !errors.Is(err, ErrClientClosed) {
		t.Fatalf("expected ErrClientClosed, got %v", err)
	}
}
