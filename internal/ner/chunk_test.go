package ner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

// fakeClient — fake источник кандидатов NER. Распознаёт заданные сущности в
// переданном тексте и возвращает кандидатов с offsets относительно chunk.
type fakeClient struct {
	mu       sync.Mutex
	calls    []string
	entities []string
}

func newFakeClient(entities ...string) *fakeClient {
	return &fakeClient{entities: entities}
}

func (f *fakeClient) Detect(_ context.Context, text string) ([]Candidate, error) {
	f.mu.Lock()
	f.calls = append(f.calls, text)
	f.mu.Unlock()
	var cands []Candidate
	for _, ent := range f.entities {
		offset := 0
		for {
			idx := strings.Index(text[offset:], ent)
			if idx < 0 {
				break
			}
			start := offset + idx
			cands = append(cands, Candidate{
				Label: LabelPER,
				Start: start,
				End:   start + len(ent),
				Text:  ent,
			})
			offset = start + len(ent)
		}
	}
	return cands, nil
}

func (f *fakeClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// testDetector создаёт детектор с fake-клиентом и заданными chunk options.
func testDetector(fc *fakeClient, opts chunkOptions) *Detector {
	return &Detector{
		client:    fc,
		validator: NewValidator(),
		cache:     NewCache(CacheOptions{}),
		chunkOpts: opts,
	}
}

// TestChunkShortTextFastPath — короткий текст идёт старым путём: один вызов
// fake-клиента, без chunking.
func TestChunkShortTextFastPath(t *testing.T) {
	fc := newFakeClient("Иван Петров")
	d := testDetector(fc, chunkOptions{chunkSize: 100, overlap: 20})
	text := "клиент Иван Петров"
	cands, err := d.detectCandidates(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if fc.callCount() != 1 {
		t.Fatalf("expected 1 call, got %d", fc.callCount())
	}
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	if text[cands[0].Start:cands[0].End] != "Иван Петров" {
		t.Fatalf("wrong offset: %q", text[cands[0].Start:cands[0].End])
	}
}

// TestChunkEntityBeforeBoundary — сущность перед границей chunk не теряется.
func TestChunkEntityBeforeBoundary(t *testing.T) {
	fc := newFakeClient("Иван Петров")
	d := testDetector(fc, chunkOptions{chunkSize: 100, overlap: 20})
	// Сущность размещена так, чтобы попасть в конец первого chunk.
	text := strings.Repeat("слово ", 20) + "клиент Иван Петров" + strings.Repeat(" слово", 20)
	cands, err := d.detectCandidates(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	if text[cands[0].Start:cands[0].End] != "Иван Петров" {
		t.Fatalf("wrong offset: %q", text[cands[0].Start:cands[0].End])
	}
}

// TestChunkEntityInOverlap — сущность в overlap не дублируется.
func TestChunkEntityInOverlap(t *testing.T) {
	fc := newFakeClient("Иван Петров")
	d := testDetector(fc, chunkOptions{chunkSize: 100, overlap: 20})
	// Много сущностей — часть попадёт в overlap соседних chunk.
	text := strings.Repeat("клиент Иван Петров, ", 20)
	cands, err := d.detectCandidates(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	// Ожидаем ровно 20 сущностей (по одной на повторение), без дублей.
	if len(cands) != 20 {
		t.Fatalf("expected 20 candidates, got %d", len(cands))
	}
	seen := map[string]bool{}
	for _, c := range cands {
		if text[c.Start:c.End] != "Иван Петров" {
			t.Fatalf("wrong entity at %d-%d: %q", c.Start, c.End, text[c.Start:c.End])
		}
		key := fmt.Sprintf("%d-%d", c.Start, c.End)
		if seen[key] {
			t.Fatalf("duplicate candidate at %s", key)
		}
		seen[key] = true
	}
}

// TestChunkEntityAfterBoundary — сущность сразу после границы chunk не теряется.
func TestChunkEntityAfterBoundary(t *testing.T) {
	fc := newFakeClient("Иван Петров")
	d := testDetector(fc, chunkOptions{chunkSize: 100, overlap: 20})
	// Сущность размещена так, чтобы попасть в начало второго chunk.
	text := strings.Repeat("слово ", 20) + "клиент Иван Петров" + strings.Repeat(" слово", 20)
	cands, err := d.detectCandidates(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	if text[cands[0].Start:cands[0].End] != "Иван Петров" {
		t.Fatalf("wrong offset: %q", text[cands[0].Start:cands[0].End])
	}
}

// TestChunkCyrillicAndEmoji — кириллица и emoji не режутся, offsets корректны.
func TestChunkCyrillicAndEmoji(t *testing.T) {
	fc := newFakeClient("Иван Петров")
	d := testDetector(fc, chunkOptions{chunkSize: 100, overlap: 20})
	// Emoji (4 байта) и кириллица (2 байта) вперемешку с сущностями.
	text := strings.Repeat("📧 клиент Иван Петров, ", 20)
	cands, err := d.detectCandidates(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 20 {
		t.Fatalf("expected 20 candidates, got %d", len(cands))
	}
	for _, c := range cands {
		if text[c.Start:c.End] != "Иван Петров" {
			t.Fatalf("wrong entity at %d-%d: %q", c.Start, c.End, text[c.Start:c.End])
		}
		// Границы должны быть валидными UTF-8.
		if !utf8.ValidString(text[c.Start:c.End]) {
			t.Fatalf("invalid UTF-8 at %d-%d", c.Start, c.End)
		}
	}
}

// TestChunkMultipleChunks — сущности в разных chunks все обнаруживаются.
func TestChunkMultipleChunks(t *testing.T) {
	fc := newFakeClient("Иван Петров")
	d := testDetector(fc, chunkOptions{chunkSize: 100, overlap: 20})
	// Достаточно длинный текст, чтобы гарантированно получить несколько chunks.
	text := strings.Repeat("клиент Иван Петров, ", 50)
	cands, err := d.detectCandidates(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if fc.callCount() < 2 {
		t.Fatalf("expected multiple chunks, got %d calls", fc.callCount())
	}
	if len(cands) != 50 {
		t.Fatalf("expected 50 candidates, got %d", len(cands))
	}
}

// TestChunkLargeText — текст минимум из 100000 whitespace-separated токенов.
// Проверяет: полное прохождение без обрезания, покрытие всех chunks, корректные
// UTF-8 offsets, сущности в начале/середине/конце с правильными глобальными
// offsets, отсутствие дублей из overlap.
func TestChunkLargeText(t *testing.T) {
	fc := newFakeClient("Иван Петров")
	d := testDetector(fc, chunkOptions{chunkSize: 8192, overlap: 512})

	// Генерируем ровно 100000 whitespace-separated токенов.
	const nTokens = 100000
	tokens := make([]string, nTokens)
	for i := range tokens {
		tokens[i] = "слово"
	}
	// Сущности в начале, середине и конце (два токена "Иван Петров").
	tokens[0], tokens[1] = "Иван", "Петров"
	mid := nTokens / 2
	tokens[mid], tokens[mid+1] = "Иван", "Петров"
	tokens[nTokens-2], tokens[nTokens-1] = "Иван", "Петров"
	text := strings.Join(tokens, " ")

	// Проверяем, что токенов действительно >= 100000.
	if got := len(strings.Fields(text)); got < 100000 {
		t.Fatalf("expected >=100000 tokens, got %d", got)
	}

	// Покрытие: все chunks покрывают исходный текст без пропусков.
	chunks := splitChunks(text, d.chunkOpts)
	if chunks[0].start != 0 {
		t.Fatalf("first chunk should start at 0")
	}
	if chunks[len(chunks)-1].end != len(text) {
		t.Fatalf("last chunk should end at len(text)")
	}
	for i := 1; i < len(chunks); i++ {
		if chunks[i].start < chunks[i-1].start {
			t.Fatalf("chunks out of order")
		}
		if chunks[i].start > chunks[i-1].end {
			t.Fatalf("gap between chunks %d and %d", i-1, i)
		}
	}

	cands, err := d.detectCandidates(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	// Ожидаем ровно 3 сущности (начало, середина, конец), без дублей.
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates, got %d (truncation?)", len(cands))
	}
	seen := map[string]bool{}
	for _, c := range cands {
		if text[c.Start:c.End] != "Иван Петров" {
			t.Fatalf("wrong entity at %d-%d: %q", c.Start, c.End, text[c.Start:c.End])
		}
		if !utf8.ValidString(text[c.Start:c.End]) {
			t.Fatalf("invalid UTF-8 at %d-%d", c.Start, c.End)
		}
		key := fmt.Sprintf("%d-%d", c.Start, c.End)
		if seen[key] {
			t.Fatalf("duplicate candidate at %s", key)
		}
		seen[key] = true
	}

	t.Logf("large text: bytes=%d tokens=%d chunks=%d candidates=%d",
		len(text), len(strings.Fields(text)), len(chunks), len(cands))
}

// TestChunkBoundariesUTF8 — границы всех chunks валидны для UTF-8.
func TestChunkBoundariesUTF8(t *testing.T) {
	opts := chunkOptions{chunkSize: 100, overlap: 20}
	text := strings.Repeat("📧 клиент Иван Петров, ", 30)
	chunks := splitChunks(text, opts)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, ch := range chunks {
		if !utf8.ValidString(ch.text) {
			t.Fatalf("chunk %d-%d is invalid UTF-8", ch.start, ch.end)
		}
	}
	// Покрытие без пропусков.
	if chunks[0].start != 0 {
		t.Fatalf("first chunk should start at 0")
	}
	if chunks[len(chunks)-1].end != len(text) {
		t.Fatalf("last chunk should end at len(text)")
	}
	for i := 1; i < len(chunks); i++ {
		if chunks[i].start < chunks[i-1].start {
			t.Fatalf("chunks out of order")
		}
		if chunks[i].start > chunks[i-1].end {
			t.Fatalf("gap between chunks %d and %d", i-1, i)
		}
	}
}

type failingClient struct {
	calls int
}

func (f *failingClient) Detect(_ context.Context, _ string) ([]Candidate, error) {
	f.calls++
	if f.calls == 2 {
		return nil, errors.New("sidecar unavailable")
	}
	return nil, nil
}

func TestChunkErrorIsPropagated(t *testing.T) {
	client := &failingClient{}
	d := &Detector{
		client:    client,
		validator: NewValidator(),
		cache:     NewCache(CacheOptions{}),
		chunkOpts: chunkOptions{chunkSize: 32, overlap: 8},
	}
	_, err := d.detectCandidates(context.Background(), strings.Repeat("длинный текст ", 20))
	if err == nil || err.Error() != "sidecar unavailable" {
		t.Fatalf("expected sidecar error, got %v", err)
	}
}

func TestChunkBoundariesUTF8WithoutSeparators(t *testing.T) {
	text := strings.Repeat("🙂", 200)
	chunks := splitChunks(text, chunkOptions{chunkSize: 101, overlap: 17})
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, ch := range chunks {
		if !utf8.ValidString(ch.text) {
			t.Fatalf("chunk %d:%d splits a UTF-8 rune", ch.start, ch.end)
		}
		if ch.text != text[ch.start:ch.end] {
			t.Fatalf("chunk offsets do not reference the source text: %d:%d", ch.start, ch.end)
		}
	}
}
