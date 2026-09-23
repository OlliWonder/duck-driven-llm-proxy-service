package detection

import (
	"context"
	"strings"
	"testing"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

func TestRegressionQuotedTicketEmailIsDetected(t *testing.T) {
	text := `Ниже оператор поддержки вставил карточку обращения: «Клиент Иван Петров, телефон +7 916 123-45-67, email ivan@example.com».`
	frags, err := NewRuleBasedDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range frags {
		if f.Type == pii.TypeEmail && text[f.Start:f.End] == "ivan@example.com" {
			return
		}
	}
	t.Fatalf("email not detected: %+v", frags)
}

func TestRegressionPublicExampleEmailAndRealEmail(t *testing.T) {
	text := `Публичная документация содержит пример JSON: {"email":"ivan@example.com"}. Реальная строка пользователя: email anna@example.net.`
	frags, err := NewEmailDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 1 || text[frags[0].Start:frags[0].End] != "anna@example.net" {
		t.Fatalf("got email spans %+v", frags)
	}
}

func TestRegressionDocumentationExamplePhoneAndRealPhone(t *testing.T) {
	text := `Публичная документация содержит пример JSON: {"phone":"+7 900 123-45-67"}. Реальная строка пользователя: телефон +7 916 123-45-67.`
	frags, err := NewPhoneDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 1 || text[frags[0].Start:frags[0].End] != "+7 916 123-45-67" {
		t.Fatalf("got phone spans %+v", frags)
	}
}

func TestRegressionBirthPlaceAndBirthDateAreSeparate(t *testing.T) {
	text := "Клиент родился в Москве 15.03.1990."
	frags, err := NewRuleBasedDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	got := map[pii.Type]string{}
	for _, f := range frags {
		got[f.Type] = text[f.Start:f.End]
	}
	if got[pii.TypeBirthPlace] != "Москве" || got[pii.TypeBirthDate] != "15.03.1990" {
		t.Fatalf("birthplace/date not split: %#v", got)
	}
}

func TestRegressionPublicAuthorAppositiveNotRuleBasedPII(t *testing.T) {
	text := `Свяжитесь с Марией Ивановой, автором доклада; её публичный рабочий адрес press@example.org.`
	frags, err := NewRuleBasedDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range frags {
		if f.Type == pii.TypeFullName && text[f.Start:f.End] == "Марией Ивановой" {
			t.Fatalf("public author name was detected: %+v", f)
		}
	}
}

func TestRegressionPublicBiographyWithLaterSameClientName(t *testing.T) {
	text := "Александр Пушкин родился в Москве. ФИО клиента: Пушкин Александр Сергеевич; дата рождения клиента 06.06.1990."
	frags, err := NewRuleBasedDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range frags {
		if f.Type == pii.TypeBirthPlace && text[f.Start:f.End] == "Москва" {
			t.Fatalf("public biographical birthplace should remain public: %+v", frags)
		}
	}
}

func TestRegressionInformalAndReversedFullNameContexts(t *testing.T) {
	cases := []struct {
		text string
		name string
	}{
		{"Клиент сказал, что его зовут Александр Пушкин", "Александр Пушкин"},
		{"Заявление поступило от Петрова Ивана Ивановича", "Петрова Ивана Ивановича"},
	}
	for _, tc := range cases {
		frags, err := NewFullNameDetector().Detect(context.Background(), tc.text)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, f := range frags {
			if f.Type == pii.TypeFullName && tc.text[f.Start:f.End] == tc.name {
				found = true
			}
		}
		if !found {
			t.Errorf("name %q was not found in %q: %+v", tc.name, tc.text, frags)
		}
	}
}

func TestRegressionSpelledOutBirthYear(t *testing.T) {
	text := "Клиент родился третьего апреля две тысячи первого года"
	frags, err := NewBirthDateDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 1 || text[frags[0].Start:frags[0].End] != "третьего апреля две тысячи первого года" {
		t.Fatalf("textual date not detected: %+v", frags)
	}
}

func TestRegressionOrganizationBranchAddressDoesNotMaskNameOnly(t *testing.T) {
	text := "Клиент Иван Петров проживает по адресу отделения банка: г. Тольятти, ул. Юбилейная, д. 31Г"
	frags, err := NewAddressDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range frags {
		value := text[f.Start:f.End]
		if f.Type == pii.TypeAddress && strings.Contains(value, "Тольятти") {
			t.Fatalf("organization branch address must remain public: %q", value)
		}
	}
}

func TestRegressionClientPipeRecordMasksUnlabeledBirthDate(t *testing.T) {
	text := "Клиент | Иван Петров | +7 927 123-45-67 | 15.03.1990"
	frags, err := NewBirthDateDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 1 || text[frags[0].Start:frags[0].End] != "15.03.1990" {
		t.Fatalf("дата в строке клиента не распознана: %+v", frags)
	}
}

func TestRegressionDocumentationTestCardRemainsPublic(t *testing.T) {
	text := "Тестовая карта 4111 1111 1111 1111 из документации"
	frags, err := NewCardNumberDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 0 {
		t.Fatalf("тестовый номер из документации ошибочно замаскирован: %+v", frags)
	}
	text = "Карта клиента 4111 1111 1111 1111"
	frags, err = NewCardNumberDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 1 || text[frags[0].Start:frags[0].End] != "4111 1111 1111 1111" {
		t.Fatalf("номер карты клиента не распознан: %+v", frags)
	}
}

func TestRegressionRegistrationAndResidenceAddressKeepsFullLabelAndValue(t *testing.T) {
	text := "Адрес регистрации и проживания клиента: Самарская область, г. Тольятти, ул. Революционная, д. 25, кв. 47. Для связи указан телефон +7 927 123-45-67."
	frags, err := NewAddressDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	want := "Самарская область, г. Тольятти, ул. Революционная, д. 25, кв. 47"
	if len(frags) != 1 || text[frags[0].Start:frags[0].End] != want {
		t.Fatalf("неверный фрагмент адреса: %+v; текст=%q", frags, text)
	}
}

func TestRegressionPINWithEmDash(t *testing.T) {
	text := "PIN-код — 4827. После проверки данные подтверждены."
	frags, err := NewPINDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 1 || text[frags[0].Start:frags[0].End] != "4827" {
		t.Fatalf("PIN-код не распознан: %+v", frags)
	}
}

func TestRegressionAddressDoesNotSwallowLaterPIN(t *testing.T) {
	text := "Адрес регистрации и проживания клиента: Самарская область, г. Тольятти, ул. Революционная, д. 25, кв. 47. PIN-код — 4827."
	frags, err := NewRuleBasedDetector().Detect(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	foundAddress, foundPIN := false, false
	for _, f := range frags {
		value := text[f.Start:f.End]
		if f.Type == pii.TypeAddress && value == "Самарская область, г. Тольятти, ул. Революционная, д. 25, кв. 47" {
			foundAddress = true
		}
		if f.Type == pii.TypePIN && value == "4827" {
			foundPIN = true
		}
	}
	if !foundAddress || !foundPIN {
		t.Fatalf("адрес или PIN потерян при объединении фрагментов: %+v", frags)
	}
}
