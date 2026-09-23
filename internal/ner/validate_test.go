package ner

import (
	"strings"
	"testing"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

func TestValidatePER(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name string
		in   string
		want []detection.Fragment
	}{
		{
			name: "poet not PII",
			in:   "поэт Александр Пушкин написал роман",
			want: nil,
		},
		{
			name: "card holder is PII",
			in:   "имя держателя карты Пушкина",
			want: []detection.Fragment{{
				Type:  pii.TypeCardHolder,
				Start: strings.Index("имя держателя карты Пушкина", "Пушкина"),
				End:   strings.Index("имя держателя карты Пушкина", "Пушкина") + len("Пушкина"),
			}},
		},
		{
			name: "client is PII",
			in:   "клиент Александр Пушкин обратился",
			want: []detection.Fragment{{
				Type:  pii.TypeFullName,
				Start: strings.Index("клиент Александр Пушкин обратился", "Александр Пушкин"),
				End:   strings.Index("клиент Александр Пушкин обратился", "Александр Пушкин") + len("Александр Пушкин"),
			}},
		},
		{
			name: "writer not PII",
			in:   "писатель Лев Толстой известен",
			want: nil,
		},
		{
			name: "clients favorite writer is not client PII",
			in:   "любимый писатель клиента Лев Толстой",
			want: nil,
		},
		{
			name: "president not PII",
			in:   "президент Владимир Путин выступил",
			want: nil,
		},
		{
			name: "no context not PII",
			in:   "Александр Пушкин пришёл",
			want: nil,
		},
		{
			name: "uppercase label",
			in:   "КЛИЕНТ Александр Пушкин",
			want: []detection.Fragment{{
				Type:  pii.TypeFullName,
				Start: strings.Index("КЛИЕНТ Александр Пушкин", "Александр Пушкин"),
				End:   strings.Index("КЛИЕНТ Александр Пушкин", "Александр Пушкин") + len("Александр Пушкин"),
			}},
		},
		{
			name: "genitive case client",
			in:   "данные клиента Александра Пушкина",
			want: []detection.Fragment{{
				Type:  pii.TypeFullName,
				Start: strings.Index("данные клиента Александра Пушкина", "Александра Пушкина"),
				End:   strings.Index("данные клиента Александра Пушкина", "Александра Пушкина") + len("Александра Пушкина"),
			}},
		},
		{
			name: "emoji before client",
			in:   "📧 клиент Александр Пушкин",
			want: []detection.Fragment{{
				Type:  pii.TypeFullName,
				Start: strings.Index("📧 клиент Александр Пушкин", "Александр Пушкин"),
				End:   strings.Index("📧 клиент Александр Пушкин", "Александр Пушкин") + len("Александр Пушкин"),
			}},
		},
		{
			name: "conflict negative wins",
			in:   "клиент и поэт Александр Пушкин",
			want: nil,
		},
		{
			name: "client then poet after comma is PII",
			in:   "клиент Александр Пушкин, известный поэт",
			want: []detection.Fragment{{
				Type:  pii.TypeFullName,
				Start: strings.Index("клиент Александр Пушкин, известный поэт", "Александр Пушкин"),
				End:   strings.Index("клиент Александр Пушкин, известный поэт", "Александр Пушкин") + len("Александр Пушкин"),
			}},
		},
		{
			name: "poet then client later not PII",
			in:   "поэт Александр Пушкин был клиентом другого банка",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Находим все PER-кандидаты в тексте (для простоты — по известным именам).
			cands := findPERCandidates(tt.in)
			got := v.Validate(tt.in, cands)
			if !fragmentsEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			// Проверяем корректность byte offsets.
			for _, f := range got {
				if f.Start < 0 || f.End > len(tt.in) || f.Start > f.End {
					t.Fatalf("fragment out of bounds: %+v", f)
				}
			}
		})
	}
}

func TestValidateLOC(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name string
		in   string
		want []detection.Fragment
	}{
		{
			name: "bank branch address not PII",
			in:   "адрес отделения банка: Москва, ул Ленина, 1",
			want: nil,
		},
		{
			name: "client address is PII",
			in:   "адрес клиента: Москва, ул Ленина, 1",
			want: []detection.Fragment{{
				Type:  pii.TypeAddress,
				Start: strings.Index("адрес клиента: Москва, ул Ленина, 1", "Москва"),
				End:   strings.Index("адрес клиента: Москва, ул Ленина, 1", "Москва") + len("Москва"),
			}},
		},
		{
			name: "registration address is PII",
			in:   "адрес регистрации: Москва, ул Ленина",
			want: []detection.Fragment{{
				Type:  pii.TypeAddress,
				Start: strings.Index("адрес регистрации: Москва, ул Ленина", "Москва"),
				End:   strings.Index("адрес регистрации: Москва, ул Ленина", "Москва") + len("Москва"),
			}},
		},
		{
			name: "birth place is PII",
			in:   "место рождения: Москва",
			want: []detection.Fragment{{
				Type:  pii.TypeBirthPlace,
				Start: strings.Index("место рождения: Москва", "Москва"),
				End:   strings.Index("место рождения: Москва", "Москва") + len("Москва"),
			}},
		},
		{
			name: "office address not PII",
			in:   "адрес офиса: Москва, ул Ленина",
			want: nil,
		},
		{
			name: "no context not PII",
			in:   "Москва, ул Ленина",
			want: nil,
		},
		{
			name: "uppercase client address",
			in:   "АДРЕС КЛИЕНТА: Москва, ул Ленина",
			want: []detection.Fragment{{
				Type:  pii.TypeAddress,
				Start: strings.Index("АДРЕС КЛИЕНТА: Москва, ул Ленина", "Москва"),
				End:   strings.Index("АДРЕС КЛИЕНТА: Москва, ул Ленина", "Москва") + len("Москва"),
			}},
		},
		{
			name: "emoji before client address",
			in:   "📍 адрес клиента: Москва",
			want: []detection.Fragment{{
				Type:  pii.TypeAddress,
				Start: strings.Index("📍 адрес клиента: Москва", "Москва"),
				End:   strings.Index("📍 адрес клиента: Москва", "Москва") + len("Москва"),
			}},
		},
		{
			name: "conflict negative wins",
			in:   "адрес клиента и отделения банка: Москва",
			want: nil,
		},
		{
			name: "negation then client address is PII",
			in:   "это не адрес отделения, а адрес клиента: Москва",
			want: []detection.Fragment{{
				Type:  pii.TypeAddress,
				Start: strings.Index("это не адрес отделения, а адрес клиента: Москва", "Москва"),
				End:   strings.Index("это не адрес отделения, а адрес клиента: Москва", "Москва") + len("Москва"),
			}},
		},
		{
			name: "two addresses separated by semicolon",
			in:   "адрес отделения: Москва; адрес клиента: Самара",
			want: []detection.Fragment{{
				Type:  pii.TypeAddress,
				Start: strings.Index("адрес отделения: Москва; адрес клиента: Самара", "Самара"),
				End:   strings.Index("адрес отделения: Москва; адрес клиента: Самара", "Самара") + len("Самара"),
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cands := findLOCCandidates(tt.in)
			got := v.Validate(tt.in, cands)
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

func TestValidateORG(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name string
		in   string
		want []detection.Fragment
	}{
		{
			name: "issuer is PII",
			in:   "кем выдан: ОУФМС России",
			want: []detection.Fragment{{
				Type:  pii.TypePassportIssuer,
				Start: strings.Index("кем выдан: ОУФМС России", "ОУФМС России"),
				End:   strings.Index("кем выдан: ОУФМС России", "ОУФМС России") + len("ОУФМС России"),
			}},
		},
		{
			name: "plain org not PII",
			in:   "компания Сбербанк работает",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cands := findORGCandidates(tt.in)
			got := v.Validate(tt.in, cands)
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

// findPERCandidates находит известные имена в тексте как PER-кандидатов.
func findPERCandidates(text string) []Candidate {
	names := []string{
		"Александр Пушкин", "Александра Пушкина", "Лев Толстой", "Владимир Путин",
		"Пушкина", "Пушкин",
	}
	return findNonOverlapping(text, names, LabelPER)
}

// findLOCCandidates находит локации в тексте как LOC-кандидатов.
func findLOCCandidates(text string) []Candidate {
	return findNonOverlapping(text, []string{"Москва", "Самара"}, LabelLOC)
}

// findORGCandidates находит организации в тексте как ORG-кандидатов.
func findORGCandidates(text string) []Candidate {
	return findNonOverlapping(text, []string{"ОУФМС России", "Сбербанк"}, LabelORG)
}

// findNonOverlapping находит все вхождения needles в text и возвращает
// непересекающиеся кандидаты (как реальный NER): при пересечении побеждает
// более длинный, при равенстве — начинающийся раньше.
func findNonOverlapping(text string, needles []string, label Label) []Candidate {
	var all []Candidate
	for _, n := range needles {
		start := 0
		for {
			idx := strings.Index(text[start:], n)
			if idx < 0 {
				break
			}
			pos := start + idx
			all = append(all, Candidate{Label: label, Start: pos, End: pos + len(n), Text: n})
			start = pos + len(n)
		}
	}
	// Сортировка по start, затем по длине убыванию.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0; j-- {
			a, b := all[j-1], all[j]
			if a.Start < b.Start || (a.Start == b.Start && a.End > b.End) {
				break
			}
			all[j-1], all[j] = b, a
		}
	}
	// Убираем пересечения.
	var out []Candidate
	for _, c := range all {
		if len(out) > 0 && c.Start < out[len(out)-1].End {
			continue
		}
		out = append(out, c)
	}
	return out
}

func fragmentsEqual(a, b []detection.Fragment) bool {
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

func TestValidateRunnerNameContexts(t *testing.T) {
	v := NewValidator()
	tests := []struct {
		text      string
		candidate string
		want      string
	}{
		{text: "Клиент Пётр Николаевич Орлов родился в г. Самара.", candidate: "Клиент Пётр Николаевич Орлов", want: "Пётр Николаевич Орлов"},
		{text: "Данные заёмщика: Пётр Николаевич Орлов.", candidate: "Пётр Николаевич Орлов", want: "Пётр Николаевич Орлов"},
		{text: "По заявлению гражданина Пётр Николаевич Орлов начата проверка.", candidate: "Пётр Николаевич Орлов", want: "Пётр Николаевич Орлов"},
	}
	for _, tt := range tests {
		start := strings.Index(tt.text, tt.candidate)
		candidate := Candidate{Label: LabelPER, Start: start, End: start + len(tt.candidate), Text: tt.candidate}
		got := v.Validate(tt.text, []Candidate{candidate})
		wantStart := strings.Index(tt.text, tt.want)
		want := []detection.Fragment{{Type: pii.TypeFullName, Start: wantStart, End: wantStart + len(tt.want)}}
		if !fragmentsEqual(got, want) {
			t.Fatalf("%q: got %+v, want %+v", tt.text, got, want)
		}
	}
}
