package ner

import (
	"context"
	"testing"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// TestDetectorIntegration проверяет полный проход: Slovnet → валидатор →
// Fragment. Требует запущенного sidecar.
func TestDetectorIntegration(t *testing.T) {
	skipIfUnavailable(t)
	d := NewDetector(NewClient(testBaseURL))
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		// wantTypes — ожидаемые типы в порядке появления (без учёта отклонённых).
		wantTypes []pii.Type
	}{
		{
			name:      "client is PII",
			in:        "клиент Александр Пушкин обратился в банк",
			wantTypes: []pii.Type{pii.TypeFullName},
		},
		{
			name:      "poet not PII",
			in:        "поэт Александр Пушкин написал роман",
			wantTypes: nil,
		},
		{
			name:      "client address is PII",
			in:        "адрес клиента: Москва, ул Ленина, 1",
			wantTypes: []pii.Type{pii.TypeAddress},
		},
		{
			name:      "bank branch address not PII",
			in:        "адрес отделения банка: Москва, ул Ленина, 1",
			wantTypes: nil,
		},
		{
			name:      "birth place is PII",
			in:        "место рождения: Москва",
			wantTypes: []pii.Type{pii.TypeBirthPlace},
		},
		{
			name:      "issuer is PII",
			in:        "кем выдан: ОУФМС России по г. Москве",
			wantTypes: []pii.Type{pii.TypePassportIssuer},
		},
		{
			name:      "two candidates in one text",
			in:        "клиент Александр Пушкин, адрес клиента: Москва",
			wantTypes: []pii.Type{pii.TypeFullName, pii.TypeAddress},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frags, err := d.Detect(ctx, tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotTypes := make([]pii.Type, 0, len(frags))
			for _, f := range frags {
				gotTypes = append(gotTypes, f.Type)
			}
			if !typesEqual(gotTypes, tt.wantTypes) {
				t.Fatalf("got types %v, want %v (frags %+v)", gotTypes, tt.wantTypes, frags)
			}
			// Проверяем byte offsets: text[start:end] должен быть непустым и в границах.
			for _, f := range frags {
				if f.Start < 0 || f.End > len(tt.in) || f.Start > f.End {
					t.Fatalf("fragment out of bounds: %+v", f)
				}
				if f.Start == f.End {
					t.Fatalf("empty fragment: %+v", f)
				}
			}
		})
	}
}

// TestDetectorByteOffsets проверяет, что после полного прохода text[start:end]
// возвращает ровно исходный фрагмент.
func TestDetectorByteOffsets(t *testing.T) {
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
	for _, f := range frags {
		if f.Start < 0 || f.End > len(text) || f.Start > f.End {
			t.Fatalf("fragment out of bounds: %+v", f)
		}
		// Фрагмент должен быть непустым и корректным UTF-8.
		_ = text[f.Start:f.End]
	}
}

func typesEqual(a, b []pii.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var _ detection.Detector = (*Detector)(nil)

func TestValidatorDiagnosticStatsAreBounded(t *testing.T) {
	d := &Detector{}
	for i := 0; i < maxStatsSamples*3; i++ {
		d.statsMu.Lock()
		d.validatorDurs = appendStat(d.validatorDurs, 0)
		d.statsMu.Unlock()
	}
	if got := len(d.ValidatorStats()); got > maxStatsSamples {
		t.Fatalf("validator stats grew past bound: %d", got)
	}
}
