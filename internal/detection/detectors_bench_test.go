package detection

import (
	"context"
	"strings"
	"testing"
)

// benchText имитирует типичный запрос: короткий текст с email и телефоном.
const benchText = "клиент Иван Петров, email ivan.petrov@example.com, тел +7 (916) 123-45-67"

// benchLongText — длинный документ с несколькими значениями, чтобы проверить
// поведение на длинных входах.
var benchLongText = strings.Repeat(benchText+" ", 100)

func BenchmarkEmailDetectShort(b *testing.B) {
	d := NewEmailDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchText)
	}
}

func BenchmarkEmailDetectLong(b *testing.B) {
	d := NewEmailDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchLongText)
	}
}

func BenchmarkPhoneDetectShort(b *testing.B) {
	d := NewPhoneDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchText)
	}
}

func BenchmarkPhoneDetectLong(b *testing.B) {
	d := NewPhoneDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchLongText)
	}
}

func BenchmarkPhoneDetectNoMatch(b *testing.B) {
	d := NewPhoneDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, "просто текст без персональных данных")
	}
}

// benchStructuredText — общий структурированный текст со всеми типами
// реквизитов: паспорт, ВУ, ИНН, карта, код подразделения.
const benchStructuredText = "паспорт 4509 123456, водительское 7712 345678, ИНН 7707083893, карта 4111111111111111, код подразделения 770-123"

func BenchmarkPassportDetect(b *testing.B) {
	d := NewPassportDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchStructuredText)
	}
}

func BenchmarkDrivingLicenseDetect(b *testing.B) {
	d := NewDrivingLicenseDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchStructuredText)
	}
}

func BenchmarkINNDetect(b *testing.B) {
	d := NewINNDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchStructuredText)
	}
}

func BenchmarkCardNumberDetect(b *testing.B) {
	d := NewCardNumberDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchStructuredText)
	}
}

func BenchmarkPassportDeptCodeDetect(b *testing.B) {
	d := NewPassportDeptCodeDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchStructuredText)
	}
}

// BenchmarkStructuredComposite прогоняет все структурированные детекторы
// последовательно на одном тексте — имитация реального hot path.
func BenchmarkStructuredComposite(b *testing.B) {
	ctx := context.Background()
	detectors := []Detector{
		NewPassportDetector(),
		NewDrivingLicenseDetector(),
		NewINNDetector(),
		NewCardNumberDetector(),
		NewPassportDeptCodeDetector(),
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, d := range detectors {
			_, _ = d.Detect(ctx, benchStructuredText)
		}
	}
}

// benchLabeledText — общий текст со всеми подписанными полями.
const benchLabeledText = "ФИО: Иванов Иван Иванович, дата рождения: 15.03.1990, гражданство: РФ, место рождения: г. Москва, кем выдан: ОУФМС России, дата выдачи: 20.05.2015, адрес: г. Москва, ул. Ленина, д. 10, держатель карты: IVAN PETROV, пин: 1234, cvv: 123"

func BenchmarkFullNameDetect(b *testing.B) {
	d := NewFullNameDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchLabeledText)
	}
}

func BenchmarkCardHolderDetect(b *testing.B) {
	d := NewCardHolderDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchLabeledText)
	}
}

func BenchmarkBirthDateDetect(b *testing.B) {
	d := NewBirthDateDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchLabeledText)
	}
}

func BenchmarkPassportIssueDateDetect(b *testing.B) {
	d := NewPassportIssueDateDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchLabeledText)
	}
}

func BenchmarkCitizenshipDetect(b *testing.B) {
	d := NewCitizenshipDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchLabeledText)
	}
}

func BenchmarkBirthPlaceDetect(b *testing.B) {
	d := NewBirthPlaceDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchLabeledText)
	}
}

func BenchmarkPassportIssuerDetect(b *testing.B) {
	d := NewPassportIssuerDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchLabeledText)
	}
}

func BenchmarkAddressDetect(b *testing.B) {
	d := NewAddressDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchLabeledText)
	}
}

func BenchmarkPINDetect(b *testing.B) {
	d := NewPINDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchLabeledText)
	}
}

func BenchmarkCVVDetect(b *testing.B) {
	d := NewCVVDetector()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = d.Detect(ctx, benchLabeledText)
	}
}

// BenchmarkLabeledComposite прогоняет все подписанные детекторы на одном тексте.
func BenchmarkLabeledComposite(b *testing.B) {
	ctx := context.Background()
	detectors := []Detector{
		NewFullNameDetector(),
		NewCardHolderDetector(),
		NewBirthDateDetector(),
		NewPassportIssueDateDetector(),
		NewCitizenshipDetector(),
		NewBirthPlaceDetector(),
		NewPassportIssuerDetector(),
		NewAddressDetector(),
		NewPINDetector(),
		NewCVVDetector(),
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, d := range detectors {
			_, _ = d.Detect(ctx, benchLabeledText)
		}
	}
}