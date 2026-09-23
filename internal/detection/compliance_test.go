package detection

import (
	"context"
	"testing"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// complianceCase — один вариант проверки детектора.
type complianceCase struct {
	// typ — ожидаемый тип ПДН (или пусто для негативного варианта).
	typ pii.Type
	// in — входной текст.
	in string
	// want — ожидаемое число фрагментов (0 для негативного).
	want int
}

// allDetectors возвращает все 17 детекторов.
func allDetectors() []Detector {
	return []Detector{
		NewFullNameDetector(),
		NewBirthDateDetector(),
		NewBirthPlaceDetector(),
		NewPassportDetector(),
		NewCitizenshipDetector(),
		NewPassportIssuerDetector(),
		NewPassportDeptCodeDetector(),
		NewPassportIssueDateDetector(),
		NewDrivingLicenseDetector(),
		NewAddressDetector(),
		NewEmailDetector(),
		NewPhoneDetector(),
		NewINNDetector(),
		NewCVVDetector(),
		NewPINDetector(),
		NewCardHolderDetector(),
		NewCardNumberDetector(),
	}
}

// detectType запускает все детекторы и возвращает фрагменты заданного типа.
func detectType(ctx context.Context, text string, typ pii.Type) []Fragment {
	var out []Fragment
	for _, d := range allDetectors() {
		frags, err := d.Detect(ctx, text)
		if err != nil {
			continue
		}
		for _, f := range frags {
			if f.Type == typ {
				out = append(out, f)
			}
		}
	}
	return out
}

// TestComplianceAllTypes — table-driven проверка всех 17 типов ПДН с
// позитивными и негативными вариантами.
func TestComplianceAllTypes(t *testing.T) {
	ctx := context.Background()

	// Кейсы по каждому типу: позитивные (want>0) и негативные (want=0).
	cases := []complianceCase{
		// --- full_name ---
		{typ: pii.TypeFullName, in: "ФИО: Иванов Иван Иванович", want: 1},
		{typ: pii.TypeFullName, in: "фио: иванов иван иванович", want: 1}, // нижний регистр
		{typ: pii.TypeFullName, in: "имя: Петров Пётр", want: 1},
		{typ: pii.TypeFullName, in: "Иванов Иван Иванович пришёл", want: 0}, // без подписи
		{typ: pii.TypeFullName, in: "поэт Александр Пушкин", want: 0},       // не ПДН

		// --- birth_date ---
		{typ: pii.TypeBirthDate, in: "дата рождения: 15.03.1990", want: 1},
		{typ: pii.TypeBirthDate, in: "дата рождения: 1990-03-15", want: 1}, // ISO
		{typ: pii.TypeBirthDate, in: "дата рождения: 15/03/1990", want: 1}, // слэш
		{typ: pii.TypeBirthDate, in: "дата рождения: пятнадцатое марта 1990 года", want: 1}, // текстом
		{typ: pii.TypeBirthDate, in: "число 15.03.1990", want: 0},          // без подписи

		// --- birth_place ---
		{typ: pii.TypeBirthPlace, in: "место рождения: г. Москва", want: 1},
		{typ: pii.TypeBirthPlace, in: "родился в г. Санкт-Петербург", want: 1},
		{typ: pii.TypeBirthPlace, in: "г. Москва", want: 0}, // без подписи

		// --- passport_series ---
		{typ: pii.TypePassportSeries, in: "паспорт 4509 123456", want: 1},
		{typ: pii.TypePassportSeries, in: "серия 4509 номер 123456", want: 1}, // серия/номер раздельно
		{typ: pii.TypePassportSeries, in: "число 4509 123456", want: 0},       // без контекста

		// --- citizenship ---
		{typ: pii.TypeCitizenship, in: "гражданство: Российская Федерация", want: 1},
		{typ: pii.TypeCitizenship, in: "гражданство: РФ", want: 1},
		{typ: pii.TypeCitizenship, in: "Российская Федерация", want: 0},

		// --- passport_issuer ---
		{typ: pii.TypePassportIssuer, in: "кем выдан: ОУФМС России по г. Москве", want: 1},
		{typ: pii.TypePassportIssuer, in: "ОУФМС России", want: 0},

		// --- passport_dept_code ---
		{typ: pii.TypePassportDeptCode, in: "код подразделения 770-123", want: 1},
		{typ: pii.TypePassportDeptCode, in: "число 770-123", want: 0},

		// --- passport_issue_date ---
		{typ: pii.TypePassportIssueDate, in: "дата выдачи: 20.05.2015", want: 1},
		{typ: pii.TypePassportIssueDate, in: "дата выдачи: 2015-05-20", want: 1}, // ISO
		{typ: pii.TypePassportIssueDate, in: "число 20.05.2015", want: 0},

		// --- driving_license ---
		{typ: pii.TypeDrivingLicense, in: "водительское удостоверение 7712 345678", want: 1},
		{typ: pii.TypeDrivingLicense, in: "число 7712 345678", want: 0},

		// --- address ---
		{typ: pii.TypeAddress, in: "адрес: г. Москва, ул. Ленина, д. 10", want: 1},
		{typ: pii.TypeAddress, in: "адрес регистрации: г. Москва, ул. Тверская", want: 1},
		{typ: pii.TypeAddress, in: "адрес клиента: г. Москва, ул. Ленина", want: 1}, // адрес клиента
		{typ: pii.TypeAddress, in: "адрес отделения банка: г. Москва", want: 0},     // не ПДН
		{typ: pii.TypeAddress, in: "г. Москва, ул. Ленина", want: 0},                // без подписи

		// --- email ---
		{typ: pii.TypeEmail, in: "почта ivan@example.com", want: 1},
		{typ: pii.TypeEmail, in: "просто текст", want: 0},

		// --- phone ---
		{typ: pii.TypePhone, in: "тел +79161234567", want: 1},
		{typ: pii.TypePhone, in: "тел: +7 (916) 123-45-67", want: 1},
		{typ: pii.TypePhone, in: "число 12345", want: 0},

		// --- inn ---
		{typ: pii.TypeINN, in: "ИНН 7707083893", want: 1},
		{typ: pii.TypeINN, in: "ИНН 500100732259", want: 1},
		{typ: pii.TypeINN, in: "ИНН 7707083894", want: 0}, // неверная контрольная сумма

		// --- cvv ---
		{typ: pii.TypeCVV, in: "cvv: 123", want: 1},
		{typ: pii.TypeCVV, in: "число 123", want: 0},

		// --- pin ---
		{typ: pii.TypePIN, in: "пин-код: 1234", want: 1},
		{typ: pii.TypePIN, in: "число 1234", want: 0},

		// --- card_holder ---
		{typ: pii.TypeCardHolder, in: "держатель карты: IVAN PETROV", want: 1},
		{typ: pii.TypeCardHolder, in: "имя держателя: иван петров", want: 1}, // нижний регистр
		{typ: pii.TypeCardHolder, in: "IVAN PETROV", want: 0},

		// --- card_number ---
		{typ: pii.TypeCardNumber, in: "карта 4111111111111111", want: 1},
		{typ: pii.TypeCardNumber, in: "номер 4532015112830366", want: 1},
		{typ: pii.TypeCardNumber, in: "число 4111111111111112", want: 0}, // невалидный Luhn без контекста
	}

	// Отчёт по типам.
	type typeReport struct {
		passed int
		failed int
	}
	reports := map[pii.Type]*typeReport{}
	var totalPass, totalFail int

	for _, tc := range cases {
		got := detectType(ctx, tc.in, tc.typ)
		ok := len(got) == tc.want
		if tc.want == 0 {
			ok = len(got) == 0
		}
		if reports[tc.typ] == nil {
			reports[tc.typ] = &typeReport{}
		}
		if ok {
			reports[tc.typ].passed++
			totalPass++
		} else {
			reports[tc.typ].failed++
			totalFail++
			t.Errorf("[%s] %q: got %d fragments, want %d (got %+v)",
				tc.typ, tc.in, len(got), tc.want, got)
		}
	}

	// Печатаем отчёт по типам.
	t.Logf("=== Compliance report (%d cases) ===", len(cases))
	for _, typ := range pii.AllTypes {
		r := reports[typ]
		if r == nil {
			t.Logf("  %-22s no cases", typ)
			continue
		}
		t.Logf("  %-22s passed=%d failed=%d", typ, r.passed, r.failed)
	}
	t.Logf("  TOTAL passed=%d failed=%d", totalPass, totalFail)
}

// TestComplianceMultiplePIIInOneSentence проверяет несколько разных ПДН в
// одном предложении.
func TestComplianceMultiplePIIInOneSentence(t *testing.T) {
	ctx := context.Background()
	text := "ФИО: Иванов Иван; дата рождения 15.03.1990; тел +79161234567; email ivan@example.com; адрес: г. Москва"

	// Собираем все фрагменты через Merge.
	var all []Fragment
	for _, d := range allDetectors() {
		frags, err := d.Detect(ctx, text)
		if err != nil {
			t.Fatalf("detector error: %v", err)
		}
		all = append(all, frags...)
	}
	merged := Merge(all)

	// Ожидаем минимум 4 разных типа.
	types := map[pii.Type]bool{}
	for _, f := range merged {
		types[f.Type] = true
	}
	if len(types) < 4 {
		t.Fatalf("expected >=4 distinct PII types, got %d: %+v", len(types), types)
	}
	// Фрагменты не должны пересекаться.
	for i := 1; i < len(merged); i++ {
		if merged[i].Start < merged[i-1].End {
			t.Fatalf("overlapping fragments: %+v", merged)
		}
	}
}

// TestComplianceCaseInsensitiveLabels проверяет подписи в верхнем и нижнем
// регистре.
func TestComplianceCaseInsensitiveLabels(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		typ pii.Type
		in  string
	}{
		{pii.TypeFullName, "ФИО: Иванов Иван"},
		{pii.TypeFullName, "фио: Иванов Иван"},
		{pii.TypeBirthDate, "ДАТА РОЖДЕНИЯ: 15.03.1990"},
		{pii.TypeBirthDate, "дата рождения: 15.03.1990"},
		{pii.TypeAddress, "АДРЕС: г. Москва"},
		{pii.TypeAddress, "адрес: г. Москва"},
		{pii.TypeCitizenship, "ГРАЖДАНСТВО: РФ"},
		{pii.TypeCitizenship, "гражданство: РФ"},
	}
	for _, tc := range cases {
		got := detectType(ctx, tc.in, tc.typ)
		if len(got) != 1 {
			t.Errorf("[%s] %q: got %d fragments, want 1", tc.typ, tc.in, len(got))
		}
	}
}

// TestComplianceAddressComponents проверяет компоненты адреса.
func TestComplianceAddressComponents(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		in string
	}{
		{"адрес: г. Москва, ул. Ленина, д. 10, кв. 5"},
		{"адрес: Москва, Ленинский проспект, 12"},
		{"адрес: г. Санкт-Петербург, Невский проспект, д. 1"},
		{"адрес регистрации: Московская область, г. Подольск, ул. Кирова"},
	}
	for _, tc := range cases {
		got := detectType(ctx, tc.in, pii.TypeAddress)
		if len(got) != 1 {
			t.Errorf("address %q: got %d fragments, want 1", tc.in, len(got))
		}
	}
}

// TestCompliancePoetVsClient проверяет «поэт» (не ПДН) против «клиент» (ПДН)
// на уровне rule-based детекторов. Эти кейсы также покрываются NER-валидатором.
func TestCompliancePoetVsClient(t *testing.T) {
	ctx := context.Background()
	// Rule-based: без подписи «фио/имя» имя не детектируется ни в одном случае.
	for _, in := range []string{"поэт Александр Пушкин", "клиент Александр Пушкин"} {
		got := detectType(ctx, in, pii.TypeFullName)
		if len(got) != 0 {
			t.Errorf("%q: rule-based full_name got %d fragments, want 0 (обрабатывается NER)", in, len(got))
		}
	}
}