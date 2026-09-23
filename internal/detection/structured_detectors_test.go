package detection

import (
	"context"
	"testing"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

func TestPassportDetect(t *testing.T) {
	d := NewPassportDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "passport with context",
			in:   "паспорт 4509 123456 выдан",
			want: []Fragment{span("паспорт 4509 123456 выдан", "4509 123456", pii.TypePassportSeries)},
		},
		{
			name: "series context",
			in:   "серия 4509 123456",
			want: []Fragment{span("серия 4509 123456", "4509 123456", pii.TypePassportSeries)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "📄 паспорт: 4509 123456",
			want: []Fragment{span("📄 паспорт: 4509 123456", "4509 123456", pii.TypePassportSeries)},
		},
		{
			name: "no context not passport",
			in:   "число 4509 123456 просто",
			want: nil,
		},
		{
			name: "driving license context not passport",
			in:   "водительское 7712 345678",
			want: nil,
		},
		{
			name: "nearest driving context wins",
			in:   "паспорт указан ранее, водительское удостоверение 7712 345678",
			want: nil,
		},
		{
			name: "wrong digit count",
			in:   "паспорт 4509 12345",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.Detect(ctx, tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !fragmentsEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			assertBounds(t, tt.in, got)
		})
	}
}

func TestDrivingLicenseDetect(t *testing.T) {
	d := NewDrivingLicenseDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "driving license with context",
			in:   "водительское удостоверение 7712 345678",
			want: []Fragment{span("водительское удостоверение 7712 345678", "7712 345678", pii.TypeDrivingLicense)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "🚗 ВУ: 7712 345678",
			want: []Fragment{span("🚗 ВУ: 7712 345678", "7712 345678", pii.TypeDrivingLicense)},
		},
		{
			name: "passport context not driving license",
			in:   "паспорт 4509 123456",
			want: nil,
		},
		{
			name: "nearest driving context wins",
			in:   "паспорт указан ранее, водительское удостоверение 7712 345678",
			want: []Fragment{span("паспорт указан ранее, водительское удостоверение 7712 345678", "7712 345678", pii.TypeDrivingLicense)},
		},
		{
			name: "no context",
			in:   "число 7712 345678",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.Detect(ctx, tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !fragmentsEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			assertBounds(t, tt.in, got)
		})
	}
}

func TestINNDetect(t *testing.T) {
	d := NewINNDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "inn 10 digits",
			in:   "ИНН 7707083893",
			want: []Fragment{span("ИНН 7707083893", "7707083893", pii.TypeINN)},
		},
		{
			name: "inn 12 digits",
			in:   "ИНН 500100732259",
			want: []Fragment{span("ИНН 500100732259", "500100732259", pii.TypeINN)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "📄 ИНН: 7707083893",
			want: []Fragment{span("📄 ИНН: 7707083893", "7707083893", pii.TypeINN)},
		},
		{
			name: "invalid checksum not inn",
			in:   "ИНН 7707083894",
			want: nil,
		},
		{
			name: "card number not inn",
			in:   "карта 4111111111111111",
			want: nil,
		},
		{
			name: "phone not inn",
			in:   "тел +79161234567",
			want: nil,
		},
		{
			name: "short number not inn",
			in:   "число 12345",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.Detect(ctx, tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !fragmentsEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			assertBounds(t, tt.in, got)
		})
	}
}

func TestCardNumberDetect(t *testing.T) {
	d := NewCardNumberDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "valid luhn card",
			in:   "карта 4111111111111111",
			want: []Fragment{span("карта 4111111111111111", "4111111111111111", pii.TypeCardNumber)},
		},
		{
			name: "valid luhn card no context",
			in:   "номер 4532015112830366",
			want: []Fragment{span("номер 4532015112830366", "4532015112830366", pii.TypeCardNumber)},
		},
		{
			name: "card with spaces",
			in:   "карта 4111 1111 1111 1111",
			want: []Fragment{span("карта 4111 1111 1111 1111", "4111 1111 1111 1111", pii.TypeCardNumber)},
		},
		{
			name: "invalid luhn with card context not card",
			in:   "карта 4111111111111112",
			want: nil,
		},
		{
			name: "invalid luhn no context not card",
			in:   "число 4111111111111112",
			want: nil,
		},
		{
			name: "cyrillic and emoji before",
			in:   "💳 карта: 4111111111111111",
			want: []Fragment{span("💳 карта: 4111111111111111", "4111111111111111", pii.TypeCardNumber)},
		},
		{
			name: "phone not card",
			in:   "тел +79161234567",
			want: nil,
		},
		{
			name: "inn not card",
			in:   "ИНН 7707083893",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.Detect(ctx, tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !fragmentsEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			assertBounds(t, tt.in, got)
		})
	}
}

func TestPassportDeptCodeDetect(t *testing.T) {
	d := NewPassportDeptCodeDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "dept code with context",
			in:   "код подразделения 770-123",
			want: []Fragment{span("код подразделения 770-123", "770-123", pii.TypePassportDeptCode)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "📄 подразделение: 770-123",
			want: []Fragment{span("📄 подразделение: 770-123", "770-123", pii.TypePassportDeptCode)},
		},
		{
			name: "no context not dept code",
			in:   "число 770-123",
			want: nil,
		},
		{
			name: "wrong format",
			in:   "подразделение 770-12",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.Detect(ctx, tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !fragmentsEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			assertBounds(t, tt.in, got)
		})
	}
}

// assertBounds проверяет, что все фрагменты лежат в границах строки.
func assertBounds(t *testing.T, text string, frags []Fragment) {
	t.Helper()
	for _, f := range frags {
		if f.Start < 0 || f.End > len(text) || f.Start > f.End {
			t.Fatalf("fragment out of bounds: %+v", f)
		}
	}
}

// TestNoTypeCollision проверяет, что один и тот же набор цифр не определяется
// одновременно как телефон, карта или ИНН.
func TestNoTypeCollision(t *testing.T) {
	ctx := context.Background()
	detectors := []Detector{
		NewPhoneDetector(),
		NewCardNumberDetector(),
		NewINNDetector(),
	}

	tests := []struct {
		name string
		in   string
	}{
		{name: "card number", in: "карта 4111111111111111"},
		{name: "inn", in: "ИНН 7707083893"},
		{name: "phone", in: "тел +79161234567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var all []Fragment
			for _, d := range detectors {
				frags, err := d.Detect(ctx, tt.in)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				all = append(all, frags...)
			}
			// После Merge не должно быть пересекающихся фрагментов.
			merged := Merge(all)
			for i := 1; i < len(merged); i++ {
				if merged[i].Start < merged[i-1].End {
					t.Fatalf("overlapping fragments after merge: %+v", merged)
				}
			}
			// Для каждого кейса должен быть ровно один фрагмент.
			if len(merged) != 1 {
				t.Fatalf("expected exactly 1 fragment, got %+v", merged)
			}
		})
	}
}
