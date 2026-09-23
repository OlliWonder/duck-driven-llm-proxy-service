package detection

import (
	"context"
	"testing"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

func TestBusinessFullNameQualifiedLabels(t *testing.T) {
	d := NewFullNameDetector()
	ctx := context.Background()
	tests := []struct {
		name  string
		text  string
		value string
	}{
		{name: "applicant", text: "ФИО заявителя: Иванов Иван Иванович", value: "Иванов Иван Иванович"},
		{name: "borrower", text: "ФИО заёмщика: Петров Пётр Петрович", value: "Петров Пётр Петрович"},
		{name: "qualified client", text: "ФИО современного клиента: Сидоров Сидор Сидорович", value: "Сидоров Сидор Сидорович"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.Detect(ctx, tt.text)
			if err != nil {
				t.Fatal(err)
			}
			want := []Fragment{span(tt.text, tt.value, pii.TypeFullName)}
			if !fragmentsEqual(got, want) {
				t.Fatalf("got %+v (%q), want %+v (%q)", got, fragmentsText(tt.text, got), want, tt.value)
			}
		})
	}
}

func TestBusinessAddressBoundariesAndContext(t *testing.T) {
	d := NewAddressDetector()
	ctx := context.Background()
	tests := []struct {
		name string
		text string
		want []Fragment
	}{
		{
			name: "stop at next sentence",
			text: "Адрес клиента: г. Москва, ул. Ленина, д. 10. Статус заявки: активна",
			want: []Fragment{span(
				"Адрес клиента: г. Москва, ул. Ленина, д. 10. Статус заявки: активна",
				"г. Москва, ул. Ленина, д. 10", pii.TypeAddress,
			)},
		},
		{
			name: "stop at following service field",
			text: "Адрес клиента: г. Москва, ул. Ленина, д. 10, статус заявки: активна",
			want: []Fragment{span(
				"Адрес клиента: г. Москва, ул. Ленина, д. 10, статус заявки: активна",
				"г. Москва, ул. Ленина, д. 10", pii.TypeAddress,
			)},
		},
		{name: "public venue", text: "Адрес публичной площадки: г. Москва, Красная площадь, д. 1", want: nil},
		{name: "organization", text: "Адрес организации: г. Москва, ул. Тверская, д. 1", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.Detect(ctx, tt.text)
			if err != nil {
				t.Fatal(err)
			}
			if !fragmentsEqual(got, tt.want) {
				t.Fatalf("got %+v (%q), want %+v", got, fragmentsText(tt.text, got), tt.want)
			}
		})
	}
}

func TestBusinessContactContext(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		det  Detector
		text string
		want []Fragment
	}{
		{
			name: "test TLD is syntactically valid",
			det:  NewEmailDetector(),
			text: "email клиента: client@example.test",
			want: []Fragment{span(
				"email клиента: client@example.test", "client@example.test", pii.TypeEmail,
			)},
		},
		{name: "shared organization mailbox", det: NewEmailDetector(), text: "Общий почтовый ящик организации: info@example.test", want: nil},
		{name: "shared hotline", det: NewPhoneDetector(), text: "Общая горячая линия организации: +7 (800) 555-35-35", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.det.Detect(ctx, tt.text)
			if err != nil {
				t.Fatal(err)
			}
			if !fragmentsEqual(got, tt.want) {
				t.Fatalf("got %+v (%q), want %+v", got, fragmentsText(tt.text, got), tt.want)
			}
		})
	}
}

func TestBusinessPassportAllZeroSeries(t *testing.T) {
	text := "Паспорт клиента 0000 560793"
	got, err := NewPassportDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	want := []Fragment{span(text, "0000 560793", pii.TypePassportSeries)}
	if !fragmentsEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func fragmentsText(text string, fragments []Fragment) string {
	values := make([]byte, 0)
	for i, fragment := range fragments {
		if i > 0 {
			values = append(values, '|')
		}
		values = append(values, text[fragment.Start:fragment.End]...)
	}
	return string(values)
}
