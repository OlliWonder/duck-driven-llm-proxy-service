package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
	"github.com/duck-driven-llm-proxy-service/internal/masking"
	"github.com/duck-driven-llm-proxy-service/internal/metrics"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
	"github.com/duck-driven-llm-proxy-service/internal/policy"
	"github.com/duck-driven-llm-proxy-service/internal/store"
)

// ErrForbidden возвращается, когда потребителю не разрешено выполнять
// запрошенную операцию.
var ErrForbidden = errors.New("forbidden")

// ErrInvalid возвращается для некорректных запросов.
var ErrInvalid = errors.New("invalid request")

// Service координирует маскирование и демаскирование для эндпоинта /process.
type Service struct {
	detector detection.Detector
	store    *store.Store
	policy   *policy.Manager
	metrics  *metrics.Metrics
	locks    *keyedMutex
}

// NewService создаёт Service.
func NewService(det detection.Detector, st *store.Store, pol *policy.Manager, m *metrics.Metrics) *Service {
	return &Service{
		detector: det,
		store:    st,
		policy:   pol,
		metrics:  m,
		locks:    newKeyedMutex(),
	}
}

// Process обрабатывает один запрос. Он выбирает между маскированием и
// демаскированием на основе состояния хранилища для payloadID, применяя
// политику потребителя.
func (s *Service) Process(ctx context.Context, payload, payloadID, consumer string) (string, error) {
	if payloadID == "" {
		return "", fmt.Errorf("%w: payload_id is required", ErrInvalid)
	}
	if !s.policy.Allowed(consumer) {
		return "", fmt.Errorf("%w: consumer %q is not allowed", ErrForbidden, consumer)
	}
	p := s.policy.For(consumer)

	// Сериализуем все операции для одного payload_id.
	s.locks.Lock(payloadID)
	defer s.locks.Unlock(payloadID)

	rec, err := s.store.Get(payloadID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return s.doMask(ctx, payload, payloadID, p)
	case err != nil:
		return "", err
	}

	original, err := s.store.Decrypt(rec.Original)
	if err != nil {
		return "", err
	}
	masked, err := s.store.Decrypt(rec.Masked)
	if err != nil {
		return "", err
	}
	mappingsBytes, err := s.store.Decrypt(rec.Mappings)
	if err != nil {
		return "", err
	}
	mappings, err := decodeMappings(mappingsBytes)
	if err != nil {
		return "", err
	}

	switch {
	case string(original) == payload:
		// Повтор оригинала → идемпотентное маскирование, возвращаем ту же маску.
		return string(masked), nil
	case string(masked) == payload:
		// Точное демаскирование: пришла та же маска.
		if !p.RestoreAllowed {
			return "", fmt.Errorf("%w: restore not allowed for consumer %q", ErrForbidden, consumer)
		}
		return string(original), nil
	default:
		// Возможно, текст был изменён LLM, но токены сохранились.
		// Восстанавливаем оригинал по токенам.
		restored, rerr := masking.Restore(payload, mappings)
		if rerr != nil {
			return "", rerr
		}
		if restored != payload {
			// Найдены токены → это демаскирование изменённого текста.
			if !p.RestoreAllowed {
				return "", fmt.Errorf("%w: restore not allowed for consumer %q", ErrForbidden, consumer)
			}
			return restored, nil
		}
		// Токенов нет и текст не совпадает ни с оригиналом, ни с маской →
		// считаем новой операцией маскирования (перезапись).
		return s.doMask(ctx, payload, payloadID, p)
	}
}

func (s *Service) doMask(ctx context.Context, payload, payloadID string, p policy.Policy) (string, error) {
	fragments, err := s.detector.Detect(ctx, payload)
	if err != nil {
		return "", fmt.Errorf("detection failed: %w", err)
	}
	filtered := filterByPolicy(fragments, p.FilterTypes())

	masker := masking.NewMasker(p.Mode)
	res, err := masker.Mask(payload, filtered)
	if err != nil {
		return "", err
	}

	encOriginal, err := s.store.Encrypt([]byte(payload))
	if err != nil {
		return "", err
	}
	encMasked, err := s.store.Encrypt([]byte(res.Masked))
	if err != nil {
		return "", err
	}
	encMappings, err := s.store.Encrypt(encodeMappings(res.Mappings))
	if err != nil {
		return "", err
	}

	s.store.Put(payloadID, store.Record{
		Original: encOriginal,
		Masked:   encMasked,
		Mappings: encMappings,
	})
	return res.Masked, nil
}

func filterByPolicy(fragments []detection.Fragment, allowed map[pii.Type]bool) []detection.Fragment {
	if len(fragments) == 0 {
		return nil
	}
	out := make([]detection.Fragment, 0, len(fragments))
	for _, f := range fragments {
		if allowed[f.Type] {
			out = append(out, f)
		}
	}
	return out
}