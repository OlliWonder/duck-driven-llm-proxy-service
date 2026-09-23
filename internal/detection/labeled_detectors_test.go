package detection

import (
	"context"
	"testing"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

func TestFullNameDetect(t *testing.T) {
	d := NewFullNameDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "full name with colon",
			in:   "ФИО: Иванов Иван Иванович",
			want: []Fragment{span("ФИО: Иванов Иван Иванович", "Иванов Иван Иванович", pii.TypeFullName)},
		},
		{
			name: "full name with dash",
			in:   "ФИО - Петров Петр Петрович",
			want: []Fragment{span("ФИО - Петров Петр Петрович", "Петров Петр Петрович", pii.TypeFullName)},
		},
		{
			name: "lowercase label",
			in:   "фио: Сидоров Сидор Сидорович",
			want: []Fragment{span("фио: Сидоров Сидор Сидорович", "Сидоров Сидор Сидорович", pii.TypeFullName)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "📄 ФИО: Иванов Иван Иванович",
			want: []Fragment{span("📄 ФИО: Иванов Иван Иванович", "Иванов Иван Иванович", pii.TypeFullName)},
		},
		{
			name: "two words",
			in:   "имя: Иванов Иван",
			want: []Fragment{span("имя: Иванов Иван", "Иванов Иван", pii.TypeFullName)},
		},
		{
			name: "no label not full name",
			in:   "Иванов Иван Иванович пришёл",
			want: nil,
		},
		{
			name: "lowercase words in signed field",
			in:   "фио: иванов иван",
			want: []Fragment{span("фио: иванов иван", "иванов иван", pii.TypeFullName)},
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

func TestCardHolderDetect(t *testing.T) {
	d := NewCardHolderDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "card holder with colon",
			in:   "держатель карты: IVAN PETROV",
			want: []Fragment{span("держатель карты: IVAN PETROV", "IVAN PETROV", pii.TypeCardHolder)},
		},
		{
			name: "card holder cyrillic",
			in:   "имя держателя: Иван Петров",
			want: []Fragment{span("имя держателя: Иван Петров", "Иван Петров", pii.TypeCardHolder)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "💳 держатель карты: IVAN PETROV",
			want: []Fragment{span("💳 держатель карты: IVAN PETROV", "IVAN PETROV", pii.TypeCardHolder)},
		},
		{
			name: "no label not card holder",
			in:   "IVAN PETROV",
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

func TestBirthDateDetect(t *testing.T) {
	d := NewBirthDateDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "birth date with colon",
			in:   "дата рождения: 15.03.1990",
			want: []Fragment{span("дата рождения: 15.03.1990", "15.03.1990", pii.TypeBirthDate)},
		},
		{
			name: "birth date short year",
			in:   "дата рождения: 15.03.90",
			want: []Fragment{span("дата рождения: 15.03.90", "15.03.90", pii.TypeBirthDate)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "📄 дата рождения: 15.03.1990",
			want: []Fragment{span("📄 дата рождения: 15.03.1990", "15.03.1990", pii.TypeBirthDate)},
		},
		{
			name: "no label not birth date",
			in:   "число 15.03.1990",
			want: nil,
		},
		{
			name: "hyphen-separated date",
			in:   "дата рождения: 15-03-1990",
			want: []Fragment{span("дата рождения: 15-03-1990", "15-03-1990", pii.TypeBirthDate)},
		},
		{
			name: "month-first hyphen-separated date",
			in:   "дата рождения: 03-15-1990",
			want: []Fragment{span("дата рождения: 03-15-1990", "03-15-1990", pii.TypeBirthDate)},
		},
		{
			name: "invalid hyphen-separated calendar date",
			in:   "дата рождения: 31-04-1990",
			want: nil,
		},
		{
			name: "public person biography not pii",
			in:   "поэт родился 6 июня 1799 года",
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

func TestPassportIssueDateDetect(t *testing.T) {
	d := NewPassportIssueDateDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "issue date with colon",
			in:   "дата выдачи: 20.05.2015",
			want: []Fragment{span("дата выдачи: 20.05.2015", "20.05.2015", pii.TypePassportIssueDate)},
		},
		{
			name: "issued date",
			in:   "выдан 20.05.2015",
			want: []Fragment{span("выдан 20.05.2015", "20.05.2015", pii.TypePassportIssueDate)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "📄 дата выдачи: 20.05.2015",
			want: []Fragment{span("📄 дата выдачи: 20.05.2015", "20.05.2015", pii.TypePassportIssueDate)},
		},
		{
			name: "no label not issue date",
			in:   "число 20.05.2015",
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

func TestCitizenshipDetect(t *testing.T) {
	d := NewCitizenshipDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "citizenship with colon",
			in:   "гражданство: Российская Федерация",
			want: []Fragment{span("гражданство: Российская Федерация", "Российская Федерация", pii.TypeCitizenship)},
		},
		{
			name: "citizenship short",
			in:   "гражданство: РФ",
			want: []Fragment{span("гражданство: РФ", "РФ", pii.TypeCitizenship)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "📄 гражданство: Россия",
			want: []Fragment{span("📄 гражданство: Россия", "Россия", pii.TypeCitizenship)},
		},
		{
			name: "no label not citizenship",
			in:   "Российская Федерация",
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

func TestBirthPlaceDetect(t *testing.T) {
	d := NewBirthPlaceDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "birth place with colon",
			in:   "место рождения: г. Москва",
			want: []Fragment{span("место рождения: г. Москва", "г. Москва", pii.TypeBirthPlace)},
		},
		{
			name: "born in",
			in:   "родился в г. Санкт-Петербург",
			want: []Fragment{span("родился в г. Санкт-Петербург", "г. Санкт-Петербург", pii.TypeBirthPlace)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "📄 место рождения: г. Москва",
			want: []Fragment{span("📄 место рождения: г. Москва", "г. Москва", pii.TypeBirthPlace)},
		},
		{
			name: "no label not birth place",
			in:   "г. Москва",
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

func TestPassportIssuerDetect(t *testing.T) {
	d := NewPassportIssuerDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "issuer with colon",
			in:   "кем выдан: ОУФМС России по г. Москве",
			want: []Fragment{span("кем выдан: ОУФМС России по г. Москве", "ОУФМС России по г. Москве", pii.TypePassportIssuer)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "📄 кем выдан: ОУФМС России",
			want: []Fragment{span("📄 кем выдан: ОУФМС России", "ОУФМС России", pii.TypePassportIssuer)},
		},
		{
			name: "no label not issuer",
			in:   "ОУФМС России по г. Москве",
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

func TestAddressDetect(t *testing.T) {
	d := NewAddressDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "address with colon",
			in:   "адрес: г. Москва, ул. Ленина, д. 10, кв. 5",
			want: []Fragment{span("адрес: г. Москва, ул. Ленина, д. 10, кв. 5", "г. Москва, ул. Ленина, д. 10, кв. 5", pii.TypeAddress)},
		},
		{
			name: "address registration",
			in:   "адрес регистрации: г. Москва, ул. Тверская, д. 1",
			want: []Fragment{span("адрес регистрации: г. Москва, ул. Тверская, д. 1", "г. Москва, ул. Тверская, д. 1", pii.TypeAddress)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "📄 адрес: г. Москва, ул. Ленина",
			want: []Fragment{span("📄 адрес: г. Москва, ул. Ленина", "г. Москва, ул. Ленина", pii.TypeAddress)},
		},
		{
			name: "no label not address",
			in:   "г. Москва, ул. Ленина",
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

func TestPINDetect(t *testing.T) {
	d := NewPINDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "pin with colon",
			in:   "пин-код: 1234",
			want: []Fragment{span("пин-код: 1234", "1234", pii.TypePIN)},
		},
		{
			name: "pin six digits",
			in:   "пин: 123456",
			want: []Fragment{span("пин: 123456", "123456", pii.TypePIN)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "💳 пин: 1234",
			want: []Fragment{span("💳 пин: 1234", "1234", pii.TypePIN)},
		},
		{
			name: "no context not pin",
			in:   "число 1234",
			want: nil,
		},
		{
			name: "too short not pin",
			in:   "пин: 12",
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

func TestCVVDetect(t *testing.T) {
	d := NewCVVDetector()
	ctx := context.Background()

	tests := []struct {
		name string
		in   string
		want []Fragment
	}{
		{
			name: "cvv with colon",
			in:   "cvv: 123",
			want: []Fragment{span("cvv: 123", "123", pii.TypeCVV)},
		},
		{
			name: "cvv code",
			in:   "код cvv: 456",
			want: []Fragment{span("код cvv: 456", "456", pii.TypeCVV)},
		},
		{
			name: "cyrillic and emoji before",
			in:   "💳 cvv: 123",
			want: []Fragment{span("💳 cvv: 123", "123", pii.TypeCVV)},
		},
		{
			name: "no context not cvv",
			in:   "число 123",
			want: nil,
		},
		{
			name: "four digit cid",
			in:   "cvv: 1234",
			want: []Fragment{span("cvv: 1234", "1234", pii.TypeCVV)},
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
