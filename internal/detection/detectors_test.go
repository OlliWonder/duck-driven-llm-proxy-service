package detection

import (
	"context"
	"strings"
	"testing"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// span возвращает байтовый диапазон [start,end) подстроки needle в text.
func span(text, needle string, typ pii.Type) Fragment {
	start := strings.Index(text, needle)
	if start < 0 {
		panic("needle not found: " + needle)
	}
	return Fragment{Type: typ, Start: start, End: start + len(needle)}
}

func TestEmailDetect(t *testing.T) {
	d := NewEmailDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "single email",
			in:   "свяжитесь с ivan@example.com пожалуйста",
			want: []Fragment{span("свяжитесь с ivan@example.com пожалуйста", "ivan@example.com", pii.TypeEmail)},
		},
		{
			name: "multiple emails",
			in:   "a@b.ru и c@d.org",
			want: []Fragment{
				span("a@b.ru и c@d.org", "a@b.ru", pii.TypeEmail),
				span("a@b.ru и c@d.org", "c@d.org", pii.TypeEmail),
			},
		},
		{
			name: "cyrillic before email",
			in:   "почта:test@example.com",
			want: []Fragment{span("почта:test@example.com", "test@example.com", pii.TypeEmail)},
		},
		{
			name: "emoji before email",
			in:   "📧 user@mail.com",
			want: []Fragment{span("📧 user@mail.com", "user@mail.com", pii.TypeEmail)},
		},
		{
			name: "no email",
			in:   "просто текст без данных",
			want: nil,
		},
		{
			name: "email with dots and plus",
			in:   "first.last+tag@sub.domain.co",
			want: []Fragment{span("first.last+tag@sub.domain.co", "first.last+tag@sub.domain.co", pii.TypeEmail)},
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
			// Проверяем, что срез text[start:end] возвращает ровно исходный фрагмент.
			for _, f := range got {
				if f.Start < 0 || f.End > len(tt.in) || f.Start > f.End {
					t.Fatalf("fragment out of bounds: %+v", f)
				}
			}
		})
	}
}

func TestPhoneDetect(t *testing.T) {
	d := NewPhoneDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "plus seven compact",
			in:   "номер +79161234567 клиента",
			want: []Fragment{span("номер +79161234567 клиента", "+79161234567", pii.TypePhone)},
		},
		{
			name: "eight compact",
			in:   "звоните 89161234567",
			want: []Fragment{span("звоните 89161234567", "89161234567", pii.TypePhone)},
		},
		{
			name: "plus seven with separators",
			in:   "тел: +7 (916) 123-45-67",
			want: []Fragment{span("тел: +7 (916) 123-45-67", "+7 (916) 123-45-67", pii.TypePhone)},
		},
		{
			name: "multiple phones",
			in:   "+79161234567 и 8-916-123-45-67",
			want: []Fragment{
				span("+79161234567 и 8-916-123-45-67", "+79161234567", pii.TypePhone),
				span("+79161234567 и 8-916-123-45-67", "8-916-123-45-67", pii.TypePhone),
			},
		},
		{
			name: "cyrillic before phone",
			in:   "клиент:📞 +79161234567",
			want: []Fragment{span("клиент:📞 +79161234567", "+79161234567", pii.TypePhone)},
		},
		{
			name: "emoji before phone",
			in:   "📞 8-916-123-45-67",
			want: []Fragment{span("📞 8-916-123-45-67", "8-916-123-45-67", pii.TypePhone)},
		},
		{
			name: "no phone",
			in:   "просто текст",
			want: nil,
		},
		{
			name: "card number not phone",
			in:   "карта 1234567890123456",
			want: nil,
		},
		{
			name: "inn not phone",
			in:   "ИНН 7707083893",
			want: nil,
		},
		{
			name: "short number not phone",
			in:   "номер 12345",
			want: nil,
		},
		{
			name: "digits attached to word not phone",
			in:   "abc89161234567",
			want: nil,
		},
		{
			name: "phone followed by digit not phone",
			in:   "+791612345678",
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
			for _, f := range got {
				if f.Start < 0 || f.End > len(tt.in) || f.Start > f.End {
					t.Fatalf("fragment out of bounds: %+v", f)
				}
			}
		})
	}
}

func fragmentsEqual(a, b []Fragment) bool {
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
