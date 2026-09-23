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

func TestBusinessReportedPIIRegressions(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		det  Detector
		text string
		want []Fragment
	}{
		{
			name: "birth date with client qualifier",
			det:  NewBirthDateDetector(),
			text: "Дата рождения клиента — 15.03.1990",
			want: []Fragment{span("Дата рождения клиента — 15.03.1990", "15.03.1990", pii.TypeBirthDate)},
		},
		{
			name: "passport issuer after issue date",
			det:  NewPassportIssuerDetector(),
			text: "паспорт выдан 18.07.2015 Отделом УФМС России по Самарской области, код подразделения 630-004",
			want: []Fragment{span(
				"паспорт выдан 18.07.2015 Отделом УФМС России по Самарской области, код подразделения 630-004",
				"Отделом УФМС России по Самарской области", pii.TypePassportIssuer,
			)},
		},
		{
			name: "full card holder label",
			det:  NewCardHolderDetector(),
			text: "Имя держателя карты: ALEXANDER IVANOV",
			want: []Fragment{span("Имя держателя карты: ALEXANDER IVANOV", "ALEXANDER IVANOV", pii.TypeCardHolder)},
		},
		{name: "email label is not address", det: NewAddressDetector(), text: "Адрес электронной почты: alexander.ivanov@example.com", want: nil},
		{name: "registration reference", det: NewAddressDetector(), text: "Адрес регистрации совпадает с фактическим", want: nil},
		{name: "registration service prose", det: NewAddressDetector(), text: "основной адрес регистрации менять не требуется", want: nil},
		{name: "delivery service prose", det: NewAddressDetector(), text: "хочет изменить адрес доставки новой карты", want: nil},
		{name: "public address reference", det: NewAddressDetector(), text: "Этот адрес является публичным адресом отделения банка", want: nil},
		{name: "public bank branch", det: NewAddressDetector(), text: "отделение банка, расположенное по адресу: г. Тольятти, ул. Юбилейная, д. 31Г", want: nil},
		{
			name: "public branch in previous sentence does not hide personal address",
			det:  NewAddressDetector(),
			text: "Отделение банка расположено рядом. Адрес клиента: г. Самара, ул. Ленина, д. 1",
			want: []Fragment{span(
				"Отделение банка расположено рядом. Адрес клиента: г. Самара, ул. Ленина, д. 1",
				"г. Самара, ул. Ленина, д. 1", pii.TypeAddress,
			)},
		},
		{
			name: "personal residence value only",
			det:  NewAddressDetector(),
			text: "Фактический адрес проживания клиента: Самарская область, г. Тольятти, ул. Революционная, д. 25, кв. 47",
			want: []Fragment{span(
				"Фактический адрес проживания клиента: Самарская область, г. Тольятти, ул. Революционная, д. 25, кв. 47",
				"Самарская область, г. Тольятти, ул. Революционная, д. 25, кв. 47", pii.TypeAddress,
			)},
		},
		{
			name: "new delivery value only",
			det:  NewAddressDetector(),
			text: "Новый адрес доставки: г. Казань, ул. Чистопольская, д. 18, кв. 92",
			want: []Fragment{span(
				"Новый адрес доставки: г. Казань, ул. Чистопольская, д. 18, кв. 92",
				"г. Казань, ул. Чистопольская, д. 18, кв. 92", pii.TypeAddress,
			)},
		},
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

func TestBusinessRepeatedEmailSurvivesCompositeOverlap(t *testing.T) {
	text := "Адрес электронной почты: alexander.ivanov@example.com. Продублировать на alexander.ivanov@example.com."
	direct, directErr := NewEmailDetector().Detect(context.Background(), text)
	if directErr != nil || len(direct) != 2 {
		t.Fatalf("direct email detector got %+v, err=%v", direct, directErr)
	}
	got, err := NewRuleBasedDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	var emails []Fragment
	for _, fragment := range got {
		if fragment.Type == pii.TypeEmail {
			emails = append(emails, fragment)
		}
	}
	if len(emails) != 2 {
		t.Fatalf("got %d email fragments (%q), want two complete emails; all=%+v", len(emails), fragmentsText(text, emails), got)
	}
	for _, fragment := range emails {
		if text[fragment.Start:fragment.End] != "alexander.ivanov@example.com" {
			t.Fatalf("partial email span %q", text[fragment.Start:fragment.End])
		}
	}
}

func TestBusinessUnusualDetectionBoundaries(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		det  Detector
		text string
		want []Fragment
	}{
		{
			name: "birth place qualifier and sentence boundary",
			det:  NewBirthPlaceDetector(),
			text: "Место рождения клиента — г. Омск. Гражданство: РФ.",
			want: []Fragment{span("Место рождения клиента — г. Омск. Гражданство: РФ.", "г. Омск", pii.TypeBirthPlace)},
		},
		{
			name: "citizenship qualifier",
			det:  NewCitizenshipDetector(),
			text: "Гражданство клиента — Российская Федерация.",
			want: []Fragment{span("Гражданство клиента — Российская Федерация.", "Российская Федерация", pii.TypeCitizenship)},
		},
		{
			name: "issuer sentence boundary",
			det:  NewPassportIssuerDetector(),
			text: "Кем выдан: ОУФМС России по г. Москве. Код подразделения: 770-001.",
			want: []Fragment{span(
				"Кем выдан: ОУФМС России по г. Москве. Код подразделения: 770-001.",
				"ОУФМС России по г. Москве", pii.TypePassportIssuer,
			)},
		},
		{name: "non document issuer", det: NewPassportIssuerDetector(), text: "Заказ выдан отделом продаж.", want: nil},
		{name: "missing address", det: NewAddressDetector(), text: "Адрес клиента: не указан.", want: nil},
		{name: "pending address", det: NewAddressDetector(), text: "Адрес проживания уточняется.", want: nil},
		{
			name: "department code spaced dash",
			det:  NewPassportDeptCodeDetector(),
			text: "код подразделения 630 - 004",
			want: []Fragment{span("код подразделения 630 - 004", "630 - 004", pii.TypePassportDeptCode)},
		},
		{name: "passport context does not cross sentence", det: NewPassportDetector(), text: "Паспорт проверен. Значение 4509 123456 относится к заявке.", want: nil},
		{name: "license context does not cross sentence", det: NewDrivingLicenseDetector(), text: "Водительское удостоверение проверено. Значение 6312 345678 относится к заявке.", want: nil},
		{name: "department context does not cross sentence", det: NewPassportDeptCodeDetector(), text: "Код подразделения проверен. Значение 630-004 относится к заявке.", want: nil},
		{name: "phone context does not cross sentence", det: NewPhoneDetector(), text: "Телефон уточнён. Значение 9271234567 относится к заявке.", want: nil},
		{
			name: "cvv card qualifier",
			det:  NewCVVDetector(),
			text: "CVV карты: 123",
			want: []Fragment{span("CVV карты: 123", "123", pii.TypeCVV)},
		},
		{
			name: "cvc2",
			det:  NewCVVDetector(),
			text: "CVC2: 987",
			want: []Fragment{span("CVC2: 987", "987", pii.TypeCVV)},
		},
		{
			name: "pin card qualifier",
			det:  NewPINDetector(),
			text: "PIN карты: 4827",
			want: []Fragment{span("PIN карты: 4827", "4827", pii.TypePIN)},
		},
		{
			name: "passport issue date full label",
			det:  NewPassportIssueDateDetector(),
			text: "Дата выдачи паспорта: 18.07.2015",
			want: []Fragment{span("Дата выдачи паспорта: 18.07.2015", "18.07.2015", pii.TypePassportIssueDate)},
		},
		{
			name: "passport issuer full label",
			det:  NewPassportIssuerDetector(),
			text: "Кем выдан паспорт: МВД России по г. Казани",
			want: []Fragment{span("Кем выдан паспорт: МВД России по г. Казани", "МВД России по г. Казани", pii.TypePassportIssuer)},
		},
		{
			name: "card holder owner label",
			det:  NewCardHolderDetector(),
			text: "Имя владельца карты: IVAN PETROV",
			want: []Fragment{span("Имя владельца карты: IVAN PETROV", "IVAN PETROV", pii.TypeCardHolder)},
		},
		{
			name: "combined security code label",
			det:  NewCVVDetector(),
			text: "CVV/CVC2: 123",
			want: []Fragment{span("CVV/CVC2: 123", "123", pii.TypeCVV)},
		},
		{
			name: "abbreviated phone label",
			det:  NewPhoneDetector(),
			text: "Тел.: 9271234567",
			want: []Fragment{span("Тел.: 9271234567", "9271234567", pii.TypePhone)},
		},
		{
			name: "department code unicode dash",
			det:  NewPassportDeptCodeDetector(),
			text: "код подразделения 630–004",
			want: []Fragment{span("код подразделения 630–004", "630–004", pii.TypePassportDeptCode)},
		},
		{
			name: "previous service sentence does not suppress issue date",
			det:  NewPassportIssueDateDetector(),
			text: "Товар возвращён. Дата выдачи паспорта: 18.07.2015.",
			want: []Fragment{span("Товар возвращён. Дата выдачи паспорта: 18.07.2015.", "18.07.2015", pii.TypePassportIssueDate)},
		},
		{name: "access rights are not driving license", det: NewDrivingLicenseDetector(), text: "Права доступа: значение 6312 345678.", want: nil},
		{name: "public person's labeled name", det: NewFullNameDetector(), text: "Имя писателя: Лев Толстой.", want: nil},
		{name: "card holder is not generic full name", det: NewFullNameDetector(), text: "Имя держателя карты: IVAN PETROV", want: nil},
		{name: "address instruction", det: NewAddressDetector(), text: "Адрес клиента: уточнить у оператора.", want: nil},
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

func TestRunnerRequiredRuleRegressions(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		det  Detector
		text string
		want []Fragment
	}{
		{name: "born label", det: NewBirthDateDetector(), text: "В анкете указано: рождён 15.03.1990.", want: []Fragment{span("В анкете указано: рождён 15.03.1990.", "15.03.1990", pii.TypeBirthDate)}},
		{name: "dob label", det: NewBirthDateDetector(), text: "DOB клиента: 01/12/1988.", want: []Fragment{span("DOB клиента: 01/12/1988.", "01/12/1988", pii.TypeBirthDate)}},
		{name: "numeric text date", det: NewBirthDateDetector(), text: "Заемщик сообщил дату рождения 5 мая 1985.", want: []Fragment{span("Заемщик сообщил дату рождения 5 мая 1985.", "5 мая 1985", pii.TypeBirthDate)}},
		{name: "quoted birth place", det: NewBirthPlaceDetector(), text: "Поле «место рождения»: г. Самара.", want: []Fragment{span("Поле «место рождения»: г. Самара.", "г. Самара", pii.TypeBirthPlace)}},
		{name: "birth information", det: NewBirthPlaceDetector(), text: "Сведения о рождении: город Казань.", want: []Fragment{span("Сведения о рождении: город Казань.", "город Казань", pii.TypeBirthPlace)}},
		{name: "citizenship information", det: NewCitizenshipDetector(), text: "Сведения о гражданстве клиента: Российская Федерация.", want: []Fragment{span("Сведения о гражданстве клиента: Российская Федерация.", "Российская Федерация", pii.TypeCitizenship)}},
		{name: "issuer latin label", det: NewPassportIssuerDetector(), text: "issuer паспорта: Отделом УФМС России по Самарской области.", want: []Fragment{span("issuer паспорта: Отделом УФМС России по Самарской области.", "Отделом УФМС России по Самарской области", pii.TypePassportIssuer)}},
		{name: "quoted issuer label", det: NewPassportIssuerDetector(), text: "В анкете поле «кем выдан»: ГУ МВД России по г. Москве.", want: []Fragment{span("В анкете поле «кем выдан»: ГУ МВД России по г. Москве.", "ГУ МВД России по г. Москве", pii.TypePassportIssuer)}},
		{name: "short department label", det: NewPassportDeptCodeDetector(), text: "КП документа: 630-004.", want: []Fragment{span("КП документа: 630-004.", "630-004", pii.TypePassportDeptCode)}},
		{name: "quoted issue date", det: NewPassportIssueDateDetector(), text: "Поле «дата выдачи»: 03.11.2020.", want: []Fragment{span("Поле «дата выдачи»: 03.11.2020.", "03.11.2020", pii.TypePassportIssueDate)}},
		{name: " оформления date", det: NewPassportIssueDateDetector(), text: "Дата оформления паспорта: 18.07.2015.", want: []Fragment{span("Дата оформления паспорта: 18.07.2015.", "18.07.2015", pii.TypePassportIssueDate)}},
		{name: "short license label", det: NewDrivingLicenseDetector(), text: "В/У: 63 12 345678.", want: []Fragment{span("В/У: 63 12 345678.", "63 12 345678", pii.TypeDrivingLicense)}},
		{name: "delivery address", det: NewAddressDetector(), text: "Доставка клиенту по адресу: г. Москва, Ленинградский проспект, д. 10, кв. 15.", want: []Fragment{span("Доставка клиенту по адресу: г. Москва, Ленинградский проспект, д. 10, кв. 15.", "г. Москва, Ленинградский проспект, д. 10, кв. 15", pii.TypeAddress)}},
		{name: "factual residence", det: NewAddressDetector(), text: "Фактическое место проживания — Самарская область, г. Тольятти, ул. Революционная, д. 25, кв. 47.", want: []Fragment{span("Фактическое место проживания — Самарская область, г. Тольятти, ул. Революционная, д. 25, кв. 47.", "Самарская область, г. Тольятти, ул. Революционная, д. 25, кв. 47", pii.TypeAddress)}},
		{name: "registration place", det: NewAddressDetector(), text: "Место регистрации клиента: г. Москва, Ленинградский проспект, д. 10, кв. 15.", want: []Fragment{span("Место регистрации клиента: г. Москва, Ленинградский проспект, д. 10, кв. 15.", "г. Москва, Ленинградский проспект, д. 10, кв. 15", pii.TypeAddress)}},
		{name: "cvv client", det: NewCVVDetector(), text: "Код CVV клиента 123.", want: []Fragment{span("Код CVV клиента 123.", "123", pii.TypeCVV)}},
		{name: "cvv field", det: NewCVVDetector(), text: "В поле CVV указано 123.", want: []Fragment{span("В поле CVV указано 123.", "123", pii.TypeCVV)}},
		{name: "pin client", det: NewPINDetector(), text: "ПИН-код клиента 4827.", want: []Fragment{span("ПИН-код клиента 4827.", "4827", pii.TypePIN)}},
		{name: "pin field", det: NewPINDetector(), text: "В поле PIN указано 0194.", want: []Fragment{span("В поле PIN указано 0194.", "0194", pii.TypePIN)}},
		{name: "pin code client", det: NewPINDetector(), text: "Код PIN клиента: 4827.", want: []Fragment{span("Код PIN клиента: 4827.", "4827", pii.TypePIN)}},
		{name: "holder snake label", det: NewCardHolderDetector(), text: "card_holder=ALEXANDER IVANOV.", want: []Fragment{span("card_holder=ALEXANDER IVANOV.", "ALEXANDER IVANOV", pii.TypeCardHolder)}},
		{name: "holder phrase", det: NewCardHolderDetector(), text: "На карте указано имя MARIA SOKOLOVA.", want: []Fragment{span("На карте указано имя MARIA SOKOLOVA.", "MARIA SOKOLOVA", pii.TypeCardHolder)}},
		{name: "holder bank card", det: NewCardHolderDetector(), text: "Имя на банковской карте: MARIA SOKOLOVA.", want: []Fragment{span("Имя на банковской карте: MARIA SOKOLOVA.", "MARIA SOKOLOVA", pii.TypeCardHolder)}},
		{name: "holder english name", det: NewCardHolderDetector(), text: "Cardholder name: MARIA SOKOLOVA.", want: []Fragment{span("Cardholder name: MARIA SOKOLOVA.", "MARIA SOKOLOVA", pii.TypeCardHolder)}},
		{name: "store phone", det: NewPhoneDetector(), text: "Контактный телефон магазина: +7 495 000-11-22.", want: nil},
		{name: "restaurant phone", det: NewPhoneDetector(), text: "Телефон ресторана +7 846 222-33-44 указан на сайте.", want: nil},
		{name: "sales mailbox", det: NewEmailDetector(), text: "Почта отдела продаж: sales@example.org.", want: nil},
		{name: "press mailbox", det: NewEmailDetector(), text: "Email пресс-службы press@example.org опубликован на сайте.", want: nil},
		{name: "example card", det: NewCardNumberDetector(), text: "Номер 4111 1111 1111 1111 приведён как общеизвестный тестовый номер карты.", want: nil},
		{name: "example cvv", det: NewCVVDetector(), text: "CVV 123 указан исключительно как пример в инструкции.", want: nil},
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

func TestRunnerMixedFieldBoundaries(t *testing.T) {
	text := "Паспорт серия 4510 номер 654321, выдан Отделом УФМС России по Самарской области 18.07.2015, код 630-004."
	got, err := NewRuleBasedDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	for typ, value := range map[pii.Type]string{
		pii.TypePassportSeries:    "4510 номер 654321",
		pii.TypePassportIssuer:    "Отделом УФМС России по Самарской области",
		pii.TypePassportIssueDate: "18.07.2015",
		pii.TypePassportDeptCode:  "630-004",
	} {
		found := false
		for _, fragment := range got {
			if fragment.Type == typ && text[fragment.Start:fragment.End] == value {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %s=%q in %+v (%q)", typ, value, got, fragmentsText(text, got))
		}
	}
}

func TestRunnerAddressStopsBeforeFollowingCard(t *testing.T) {
	text := "Клиент подтвердил адрес проживания г. Москва, Ленинградский проспект, д. 10, кв. 15 и номер карты 5555 5555 5555 4444."
	got, err := NewRuleBasedDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	want := map[pii.Type]string{
		pii.TypeAddress:    "г. Москва, Ленинградский проспект, д. 10, кв. 15",
		pii.TypeCardNumber: "5555 5555 5555 4444",
	}
	for typ, value := range want {
		matched := false
		for _, fragment := range got {
			if fragment.Type == typ && text[fragment.Start:fragment.End] == value {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("missing %s=%q in %+v (%q)", typ, value, got, fragmentsText(text, got))
		}
	}
}
