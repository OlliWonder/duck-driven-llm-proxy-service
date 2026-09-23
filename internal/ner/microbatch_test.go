package ner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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

// TestWorkerCountControlsConcurrentBatches проверяет, что число воркеров
// microbatcher реально задаёт максимальное число одновременно выполняющихся
// /ner/batch HTTP-запросов: с Workers=N sidecar может видеть до N запросов
// in flight. Использует httptest-сервер, который блокирует обработку, чтобы
// накопить параллельные запросы.
func TestWorkerCountControlsConcurrentBatches(t *testing.T) {
	const workers = 6

	var mu sync.Mutex
	inflight := 0
	maxInflight := 0
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		inflight++
		if inflight > maxInflight {
			maxInflight = inflight
		}
		mu.Unlock()

		<-release // блокируем, чтобы запросы накапливались параллельно

		mu.Lock()
		inflight--
		mu.Unlock()

		// Возвращаем столько результатов, сколько текстов в batch.
		var req struct {
			Texts []string `json:"texts"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		results := make([]map[string]any, len(req.Texts))
		for i := range results {
			results[i] = map[string]any{"spans": []any{}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	defer server.Close()

	c := NewClientWithOptions(server.URL, Options{
		MaxBatch:  16,
		MaxWait:   2 * time.Millisecond,
		QueueSize: 256,
		Workers:   workers,
	})
	defer c.Close()

	// Отправляем достаточно запросов, чтобы сформировалось >= workers batch
	// (каждый batch до maxBatch текстов). Тогда все воркеры должны быть заняты
	// одновременно.
	const n = workers * 16
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := c.Detect(ctx, "клиент Иван Петров "+itoa(i)); err != nil {
				t.Errorf("Detect error: %v", err)
			}
		}(i)
	}

	// Ждём, пока все воркеры займут свои слоты (каждый держит один batch).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		cur := inflight
		mu.Unlock()
		if cur >= workers {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	close(release)
	wg.Wait()

	mu.Lock()
	got := maxInflight
	mu.Unlock()

	if got < workers {
		t.Fatalf("expected at least %d concurrent /ner/batch requests, got %d", workers, got)
	}
}

// TestConcurrentBatchesAcrossEndpoints проверяет, что в режиме нескольких
// endpoints (PII_NER_ENDPOINTS) несколько batch-запросов реально выполняются
// одновременно и распределяются между разными sidecar: с Workers=N оба
// endpoint одновременно держат in-flight batch-запросы.
func TestConcurrentBatchesAcrossEndpoints(t *testing.T) {
	const workers = 6

	var mu sync.Mutex
	inflight1, inflight2 := 0, 0
	bothInflight := false
	release1 := make(chan struct{})
	release2 := make(chan struct{})

	mkHandler := func(inflight *int, release chan struct{}) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			*inflight++
			if inflight1 > 0 && inflight2 > 0 {
				bothInflight = true
			}
			mu.Unlock()

			<-release

			mu.Lock()
			*inflight--
			mu.Unlock()

			var req struct {
				Texts []string `json:"texts"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			results := make([]map[string]any, len(req.Texts))
			for i := range results {
				results[i] = map[string]any{"spans": []any{}}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
		}
	}

	s1 := httptest.NewServer(mkHandler(&inflight1, release1))
	defer s1.Close()
	s2 := httptest.NewServer(mkHandler(&inflight2, release2))
	defer s2.Close()

	c := NewClientWithEndpoints([]string{s1.URL, s2.URL}, Options{
		MaxBatch:  16,
		MaxWait:   2 * time.Millisecond,
		QueueSize: 256,
		Workers:   workers,
	})
	defer c.Close()

	// Достаточно запросов, чтобы сформировалось >= workers batch.
	const n = workers * 16
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := c.Detect(ctx, "клиент Иван Петров "+itoa(i)); err != nil {
				t.Errorf("Detect error: %v", err)
			}
		}(i)
	}

	// Ждём, пока оба endpoint одновременно держат in-flight batch.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		both := bothInflight
		mu.Unlock()
		if both {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	close(release1)
	close(release2)
	wg.Wait()

	mu.Lock()
	got := bothInflight
	mu.Unlock()

	if !got {
		t.Fatalf("expected both endpoints to hold in-flight batch requests simultaneously")
	}
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

// batchEchoHandler возвращает столько результатов, сколько текстов в batch.
func batchEchoHandler(t *testing.T, hit *int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if hit != nil {
			atomic.AddInt32(hit, 1)
		}
		var req struct {
			Texts []string `json:"texts"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		results := make([]map[string]any, len(req.Texts))
		for i := range results {
			results[i] = map[string]any{"spans": []any{}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	}
}

// TestRoundRobinDistributesBatches проверяет, что batch-запросы распределяются
// между несколькими endpoints по round-robin: оба sidecar получают запросы.
func TestRoundRobinDistributesBatches(t *testing.T) {
	var hits1, hits2 int32
	s1 := httptest.NewServer(batchEchoHandler(t, &hits1))
	defer s1.Close()
	s2 := httptest.NewServer(batchEchoHandler(t, &hits2))
	defer s2.Close()

	c := NewClientWithEndpoints([]string{s1.URL, s2.URL}, Options{
		MaxBatch:  16,
		MaxWait:   2 * time.Millisecond,
		QueueSize: 256,
		Workers:   2,
	})
	defer c.Close()

	ctx := context.Background()
	for i := 0; i < 40; i++ {
		if _, err := c.Detect(ctx, "клиент Иван Петров "+itoa(i)); err != nil {
			t.Fatalf("Detect error: %v", err)
		}
	}

	if hits1 == 0 || hits2 == 0 {
		t.Fatalf("expected both endpoints to receive batches, got hits1=%d hits2=%d", hits1, hits2)
	}
}

// TestUnavailableReplicaFailover проверяет, что при недоступной одной реплике
// запросы всё равно успешно обрабатываются через другую (failover).
func TestUnavailableReplicaFailover(t *testing.T) {
	var hits int32
	s1 := httptest.NewServer(batchEchoHandler(t, &hits))
	defer s1.Close()

	// Второй endpoint указывает на закрытый порт (недоступная реплика).
	dead := httptest.NewServer(batchEchoHandler(t, nil))
	deadURL := dead.URL
	dead.Close()

	c := NewClientWithEndpoints([]string{s1.URL, deadURL}, Options{
		MaxBatch:  16,
		MaxWait:   2 * time.Millisecond,
		QueueSize: 256,
		Workers:   2,
	})
	defer c.Close()

	ctx := context.Background()
	for i := 0; i < 20; i++ {
		if _, err := c.Detect(ctx, "клиент Иван Петров "+itoa(i)); err != nil {
			t.Fatalf("Detect error with one replica down: %v", err)
		}
	}
	if hits == 0 {
		t.Fatalf("expected live replica to serve requests, got hits=%d", hits)
	}
}
