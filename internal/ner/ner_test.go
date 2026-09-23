package ner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRequiredNERFailureIsNotEmptySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	defer client.Close()
	fragments, err := NewDetector(client).Detect(context.Background(), "клиент Иван Петров")
	if err == nil {
		t.Fatalf("expected malformed required NER response to fail, got fragments %+v", fragments)
	}
}

func TestClientNormalizesEndpointTrailingSlash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ner/batch" {
			t.Errorf("unexpected path: %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"spans":[]}]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL + "/")
	defer client.Close()
	if _, err := client.Detect(context.Background(), "обычный текст"); err != nil {
		t.Fatalf("Detect: %v", err)
	}
}

const testBaseURL = "http://127.0.0.1:8090"

// skipIfUnavailable пропускает тест, если sidecar не запущен.
func skipIfUnavailable(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(testBaseURL + "/healthz")
	if err != nil {
		t.Skipf("NER sidecar недоступен (%v), пропускаю", err)
	}
	resp.Body.Close()
}

func TestDetectBasic(t *testing.T) {
	skipIfUnavailable(t)
	c := NewClient(testBaseURL)
	ctx := context.Background()

	text := "Президент Франции Эмманюэль Макрон встретился с канцлером ФРГ Ангелой Меркель в Берлине."
	cands, err := c.Detect(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) == 0 {
		t.Fatal("expected at least one candidate")
	}
	// Проверяем, что байтовые позиции корректны: text[start:end] == Text.
	for _, cand := range cands {
		if cand.Start < 0 || cand.End > len(text) || cand.Start > cand.End {
			t.Fatalf("candidate out of bounds: %+v", cand)
		}
		if got := text[cand.Start:cand.End]; got != cand.Text {
			t.Fatalf("byte slice mismatch: got %q, want %q (cand %+v)", got, cand.Text, cand)
		}
	}
}

func TestDetectMultipleEntities(t *testing.T) {
	skipIfUnavailable(t)
	c := NewClient(testBaseURL)
	ctx := context.Background()

	text := "Иван Петров работает в Сбербанке в Москве, а Анна Сидорова живёт в Санкт-Петербурге."
	cands, err := c.Detect(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) < 4 {
		t.Fatalf("expected at least 4 entities, got %d: %+v", len(cands), cands)
	}
	// Проверяем корректность позиций.
	for _, cand := range cands {
		if got := text[cand.Start:cand.End]; got != cand.Text {
			t.Fatalf("byte slice mismatch: got %q, want %q", got, cand.Text)
		}
	}
}

func TestDetectCyrillicAndEmoji(t *testing.T) {
	skipIfUnavailable(t)
	c := NewClient(testBaseURL)
	ctx := context.Background()

	text := "📧 клиент Иван Петров из Москвы написал письмо."
	cands, err := c.Detect(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) == 0 {
		t.Fatal("expected at least one candidate")
	}
	// Emoji перед текстом не должен ломать байтовые позиции.
	for _, cand := range cands {
		if got := text[cand.Start:cand.End]; got != cand.Text {
			t.Fatalf("byte slice mismatch: got %q, want %q", got, cand.Text)
		}
	}
}

func TestDetectCaseInsensitive(t *testing.T) {
	skipIfUnavailable(t)
	c := NewClient(testBaseURL)
	ctx := context.Background()

	// Разные регистры и падежи имён.
	texts := []string{
		"ИВАН ПЕТРОВ пришёл",
		"письмо от Ивана Петрова",
		"Ивану Петрову позвонили",
	}
	for _, text := range texts {
		cands, err := c.Detect(ctx, text)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", text, err)
		}
		for _, cand := range cands {
			if got := text[cand.Start:cand.End]; got != cand.Text {
				t.Fatalf("byte slice mismatch for %q: got %q, want %q", text, got, cand.Text)
			}
		}
	}
}

func TestDetectNoEntities(t *testing.T) {
	skipIfUnavailable(t)
	c := NewClient(testBaseURL)
	ctx := context.Background()

	text := "обычный текст без имён и организаций"
	cands, err := c.Detect(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Модель может не найти сущностей — это допустимо, главное без ошибок.
	_ = cands
}

// TestTimingDataRead проверяет, что служебные timing-данные sidecar корректно
// читаются и не ломают обычный Detect.
func TestTimingDataRead(t *testing.T) {
	skipIfUnavailable(t)
	c := NewClient(testBaseURL)
	defer c.Close()
	ctx := context.Background()

	// Несколько конкурентных Detect, чтобы собрать хотя бы один batch.
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			text := "клиент Иван Петров " + itoa(i) + ", адрес клиента: Москва"
			cands, err := c.Detect(ctx, text)
			if err != nil {
				t.Errorf("Detect(%q) error: %v", text, err)
				return
			}
			if len(cands) == 0 {
				t.Errorf("Detect(%q): no candidates", text)
			}
		}(i)
	}
	wg.Wait()

	stats := c.Stats()
	if len(stats.BatchSizes) == 0 {
		t.Fatal("expected at least one batch")
	}
	// Timing-данные должны быть собраны для каждого batch.
	if len(stats.SidecarMap) != len(stats.BatchSizes) {
		t.Fatalf("SidecarMap len %d != BatchSizes len %d", len(stats.SidecarMap), len(stats.BatchSizes))
	}
	if len(stats.SidecarTotal) != len(stats.BatchSizes) {
		t.Fatalf("SidecarTotal len %d != BatchSizes len %d", len(stats.SidecarTotal), len(stats.BatchSizes))
	}
	if len(stats.HTTPRTTs) != len(stats.BatchSizes) {
		t.Fatalf("HTTPRTTs len %d != BatchSizes len %d", len(stats.HTTPRTTs), len(stats.BatchSizes))
	}
	if len(stats.QueueWaits) == 0 {
		t.Fatal("expected queue wait samples")
	}
	// Значения должны быть неотрицательными и разумными.
	for i, m := range stats.SidecarMap {
		if m < 0 {
			t.Fatalf("SidecarMap[%d] negative: %v", i, m)
		}
	}
	for i, tot := range stats.SidecarTotal {
		if tot < 0 {
			t.Fatalf("SidecarTotal[%d] negative: %v", i, tot)
		}
	}
	// SidecarTotal (map + сериализация) не должен быть меньше map.
	for i := range stats.SidecarMap {
		if stats.SidecarTotal[i] < stats.SidecarMap[i] {
			t.Fatalf("batch %d: SidecarTotal %v < SidecarMap %v", i, stats.SidecarTotal[i], stats.SidecarMap[i])
		}
	}
}

// TestDetectorValidatorTiming проверяет, что время валидатора собирается и
// обычный Detect не ломается.
func TestDetectorValidatorTiming(t *testing.T) {
	skipIfUnavailable(t)
	d := NewDetector(NewClient(testBaseURL))
	ctx := context.Background()

	text := "клиент Александр Пушкин, адрес клиента: Москва"
	frags, err := d.Detect(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(frags) == 0 {
		t.Fatal("expected fragments")
	}
	durs := d.ValidatorStats()
	if len(durs) != 1 {
		t.Fatalf("expected 1 validator sample, got %d", len(durs))
	}
	if durs[0] < 0 {
		t.Fatalf("validator duration negative: %v", durs[0])
	}
}
