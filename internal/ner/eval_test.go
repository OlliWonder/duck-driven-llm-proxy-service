package ner

import (
	"context"
	"strings"
	"testing"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// TestProductionDetectorRequiredCases is intentionally a small integration
// regression suite. Independent quality scoring belongs to the separately
// labelled testdata corpus and must not be inferred from these cases.
func TestProductionDetectorRequiredCases(t *testing.T) {
	skipIfUnavailable(t)
	client := NewClient(testBaseURL)
	defer client.Close()
	detector := NewProductionDetector(client)

	type expectedSpan struct {
		typ  pii.Type
		text string
	}
	tests := []struct {
		name string
		text string
		want []expectedSpan
	}{
		{name: "private person", text: "клиент Александр Пушкин", want: []expectedSpan{{pii.TypeFullName, "Александр Пушкин"}}},
		{name: "public person", text: "поэт Александр Пушкин", want: nil},
		{name: "birth date mdy", text: "дата рождения: 03.15.1990", want: []expectedSpan{{pii.TypeBirthDate, "03.15.1990"}}},
		{name: "birth date ydm", text: "дата рождения: 1990.15.03", want: []expectedSpan{{pii.TypeBirthDate, "1990.15.03"}}},
		{name: "text date", text: "родился 7 мая 1988 года", want: []expectedSpan{{pii.TypeBirthDate, "7 мая 1988 года"}}},
		{name: "birth place", text: "место рождения: г. Казань", want: []expectedSpan{{pii.TypeBirthPlace, "г. Казань"}}},
		{name: "passport", text: "паспорт 4509 123456", want: []expectedSpan{{pii.TypePassportSeries, "4509 123456"}}},
		{name: "citizenship", text: "гражданство: Российская Федерация", want: []expectedSpan{{pii.TypeCitizenship, "Российская Федерация"}}},
		{name: "issuer", text: "кем выдан: ОУФМС России по г. Москве", want: []expectedSpan{{pii.TypePassportIssuer, "ОУФМС России по г. Москве"}}},
		{name: "department", text: "код подразделения 770-123", want: []expectedSpan{{pii.TypePassportDeptCode, "770-123"}}},
		{name: "issue date", text: "дата выдачи: 2015.20.05", want: []expectedSpan{{pii.TypePassportIssueDate, "2015.20.05"}}},
		{name: "driving license", text: "ВУ: 7712 345678", want: []expectedSpan{{pii.TypeDrivingLicense, "7712 345678"}}},
		{name: "client address", text: "адрес клиента: г. Москва, ул. Ленина, 1", want: []expectedSpan{{pii.TypeAddress, "г. Москва, ул. Ленина, 1"}}},
		{name: "bank address", text: "адрес отделения банка: г. Москва", want: nil},
		{name: "email and phone", text: "email a.b+c@example.org, телефон +7 (916) 123-45-67", want: []expectedSpan{{pii.TypeEmail, "a.b+c@example.org"}, {pii.TypePhone, "+7 (916) 123-45-67"}}},
		{name: "inn", text: "ИНН 7707083893", want: []expectedSpan{{pii.TypeINN, "7707083893"}}},
		{name: "secrets", text: "CVV 123; PIN 4321", want: []expectedSpan{{pii.TypeCVV, "123"}, {pii.TypePIN, "4321"}}},
		{name: "card", text: "держатель карты: IVAN PETROV; номер 4111 1111 1111 1111", want: []expectedSpan{{pii.TypeCardHolder, "IVAN PETROV"}, {pii.TypeCardNumber, "4111 1111 1111 1111"}}},
		{name: "unicode offsets", text: "🙂 клиент Мария Орлова, email maria@example.ru", want: []expectedSpan{{pii.TypeFullName, "Мария Орлова"}, {pii.TypeEmail, "maria@example.ru"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detector.Detect(context.Background(), tt.text)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			want := make([]detection.Fragment, 0, len(tt.want))
			for _, expected := range tt.want {
				start := strings.Index(tt.text, expected.text)
				if start < 0 {
					t.Fatalf("bad test span %q", expected.text)
				}
				want = append(want, detection.Fragment{Type: expected.typ, Start: start, End: start + len(expected.text)})
			}
			assertFragments(t, got, want)
		})
	}
}

func assertFragments(t *testing.T, got, want []detection.Fragment) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	}
}
