// NER Detector: связывает Slovnet NER (источник кандидатов PER/LOC/ORG) с
// контекстным валидатором и возвращает готовые detection.Fragment.
//
// Поток: Slovnet возвращает сырые кандидаты PER/LOC/ORG → валидатор принимает
// или отклоняет каждого кандидата на основе контекста → только после этого
// создаются обычные Fragment. Сырые PER/LOC/ORG никогда не попадают дальше
// напрямую.
package ner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
)

// candidateSource — источник сырых кандидатов NER. Реализуется *Client
// (реальный microbatcher) и fake-клиентом в тестах.
type candidateSource interface {
	Detect(ctx context.Context, text string) ([]Candidate, error)
}

// Detector реализует detection.Detector поверх NER sidecar и валидатора.
type Detector struct {
	client    candidateSource
	validator *Validator
	cache     *Cache
	chunkOpts chunkOptions

	// Статистика времени валидатора (не влияет на логику).
	statsMu       sync.Mutex
	validatorDurs []time.Duration
}

// NewDetector создаёт NER-детектор с кешем по умолчанию.
func NewDetector(client *Client) *Detector {
	return NewDetectorWithCache(client, NewCache(CacheOptions{}))
}

// NewProductionDetector объединяет все 17 правиловых детекторов с локальным
// NER. Жизненным циклом клиента управляет сервис: при остановке он должен
// вызвать Client.Close.
func NewProductionDetector(client *Client) detection.Detector {
	return detection.NewPreferredDetector(detection.NewRuleBasedDetector(), NewDetector(client))
}

// NewDetectorWithCache создаёт NER-детектор с заданным кешем.
func NewDetectorWithCache(client *Client, cache *Cache) *Detector {
	return &Detector{
		client:    client,
		validator: NewValidator(),
		cache:     cache,
		chunkOpts: defaultChunkOptions,
	}
}

// Cache возвращает кеш детектора (для диагностики).
func (d *Detector) Cache() *Cache {
	return d.cache
}

// ValidatorStats возвращает копию замеров времени валидатора.
func (d *Detector) ValidatorStats() []time.Duration {
	d.statsMu.Lock()
	defer d.statsMu.Unlock()
	return append([]time.Duration(nil), d.validatorDurs...)
}

// ResetValidatorStats очищает замеры времени валидатора. Используется между
// режимами нагрузки, чтобы результаты не смешивались.
func (d *Detector) ResetValidatorStats() {
	d.statsMu.Lock()
	defer d.statsMu.Unlock()
	d.validatorDurs = d.validatorDurs[:0]
}

// Detect возвращает фрагменты ПДН. Результат кешируется по хешу текста и
// версии детектора; одинаковые одновременные запросы coalesce (singleflight),
// чтобы не запускать повторный NER inference. Длинные тексты разбиваются на
// части с overlap. Сырые кандидаты NER проходят через валидатор, который
// присваивает тип pii.Type или отклоняет кандидата.
func (d *Detector) Detect(ctx context.Context, text string) ([]detection.Fragment, error) {
	return d.cache.Get(ctx, text, func() ([]detection.Fragment, error) {
		cands, err := d.detectCandidates(ctx, text)
		if err != nil {
			return nil, err
		}
		if err := validateCandidates(text, cands); err != nil {
			return nil, err
		}
		vStart := time.Now()
		frags := d.validator.Validate(text, cands)
		d.statsMu.Lock()
		d.validatorDurs = appendStat(d.validatorDurs, time.Since(vStart))
		d.statsMu.Unlock()
		return frags, nil
	})
}

func validateCandidates(text string, candidates []Candidate) error {
	for index, candidate := range candidates {
		if candidate.Label != LabelPER && candidate.Label != LabelLOC && candidate.Label != LabelORG {
			return fmt.Errorf("ner: invalid candidate %d label %q", index, candidate.Label)
		}
		if candidate.Start < 0 || candidate.End <= candidate.Start || candidate.End > len(text) ||
			text[candidate.Start:candidate.End] != candidate.Text {
			return fmt.Errorf("ner: invalid candidate %d span %d:%d", index, candidate.Start, candidate.End)
		}
	}
	return nil
}

// detectCandidates возвращает сырые кандидаты NER с байтовыми смещениями в
// исходном тексте. Короткие тексты идут старым быстрым путём без chunking;
// длинные разбиваются на части с overlap, offsets переводятся обратно в
// исходный текст, дубликаты из overlap удаляются.
func (d *Detector) detectCandidates(ctx context.Context, text string) ([]Candidate, error) {
	// Быстрый путь для коротких текстов.
	if len(text) <= d.chunkOpts.chunkSize {
		return d.client.Detect(ctx, text)
	}

	chunks := splitChunks(text, d.chunkOpts)
	var all []Candidate
	seen := make(map[candKey]bool)
	for _, ch := range chunks {
		cands, err := d.client.Detect(ctx, ch.text)
		if err != nil {
			return nil, err
		}
		for _, c := range cands {
			// Переводим offsets из chunk в исходный текст.
			orig := Candidate{
				Label: c.Label,
				Start: ch.start + c.Start,
				End:   ch.start + c.End,
				Text:  c.Text,
			}
			key := candKey{label: orig.Label, start: orig.Start, end: orig.End}
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, orig)
		}
	}
	return all, nil
}

// candKey — ключ для дедупликации кандидатов из overlap.
type candKey struct {
	label Label
	start int
	end   int
}
