package detection

import (
	"context"
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
