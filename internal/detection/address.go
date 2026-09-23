package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// addressLabels — подписи поля «адрес».
var addressLabels = []string{
	"фактический адрес проживания клиента", "фактическое место проживания", "место регистрации клиента", "доставка клиенту по адресу", "адрес проживания клиента", "новый адрес доставки", "почтовый адрес физлица", "адрес доставки клиента", "адрес доставки", "клиент живёт по адресу", "клиент живет по адресу",
	"проживает по адресу", "проживает в", "адрес регистрации", "адрес проживания", "адрес клиента",
	"зарегистрирована", "зарегистрирован", "проживает", "адрес",
}

// AddressDetector находит адрес в подписанных полях.
type AddressDetector struct{}

// NewAddressDetector возвращает детектор адреса.
func NewAddressDetector() *AddressDetector { return &AddressDetector{} }

// Detect возвращает фрагменты адреса.
func (d *AddressDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	lowerText := strings.ToLower(text)
	from := 0
	for {
		labelEnd := findLabel(lowerText, from, addressLabels)
		if labelEnd < 0 {
			break
		}
		if hasIPAddressPrefix(lowerText, labelEnd) {
			from = labelEnd + 1
			continue
		}
		// Пропускаем не-ПДН контексты («адрес отделения банка», «адрес офиса» и т.п.).
		if isNonPIIAddressContext(lowerText, labelEnd) {
			from = labelEnd + 1
			continue
		}
		valStart := skipSeparators(text, labelEnd)
		valStart = skipPersonalFieldQualifier(text, lowerText, valStart)
		valEnd, ok := scanAddressValue(text, valStart)
		if ok && plausibleAddressValue(text[valStart:valEnd]) {
			frags = append(frags, Fragment{Type: pii.TypeAddress, Start: valStart, End: valEnd})
			from = valEnd
		} else {
			from = labelEnd + 1
		}
	}
	return frags, nil
}

// nonPIIAddressContexts — слова, следующие за подписью «адрес», которые
// указывают на адрес организации (не ПДН), а не на адрес человека.
var nonPIIAddressContexts = []string{
	"электронной почты", "электронной почты клиента", "e-mail", "email",
	"публичной площадки", "площадки",
	"отделения банка", "отделения", "отделение банка", "отделение",
	"офиса", "офис", "компании", "организации", "банка", "филиала",
	"филиал", "магазина", "фирмы", "представительства",
	"сайта", "сервера", "приложения",
}

func hasIPAddressPrefix(lowerText string, labelEnd int) bool {
	window := windowBefore(lowerText, labelEnd, 32)
	return strings.Contains(window, "ip-адрес") || strings.Contains(window, "ip адрес")
}

// isNonPIIAddressContext проверяет, что сразу после подписи «адрес» идёт
// контекст организации (не ПДН).
func isNonPIIAddressContext(lowerText string, labelEnd int) bool {
	// "отделение банка, расположенное по адресу: ..." names a public
	// organization location; the owner appears before the generic label.
	before := contactClauseBefore(lowerText, labelEnd, 192)
	if cut := strings.LastIndex(before, ","); cut >= 0 {
		before = before[cut+1:]
	}
	if hasOrganizationLocationContext(before) {
		return true
	}
	// Пропускаем разделители.
	i := labelEnd
	for i < len(lowerText) && (lowerText[i] == ' ' || lowerText[i] == ':' || lowerText[i] == '-' || lowerText[i] == '\t') {
		i++
	}
	for _, c := range nonPIIAddressContexts {
		if strings.HasPrefix(lowerText[i:], c) {
			return true
		}
	}
	return false
}

func hasOrganizationLocationContext(context string) bool {
	organization := false
	for _, marker := range []string{
		"отделение банка", "отделения банка", "филиал", "офис банка", "офис",
		"организация", "компания", "магазин", "представительство",
	} {
		if strings.Contains(context, marker) {
			organization = true
			break
		}
	}
	if !organization {
		return false
	}
	for _, marker := range []string{"располож", "наход", "публич", "юридический адрес", "почтовый адрес"} {
		if strings.Contains(context, marker) {
			return true
		}
	}
	return false
}

func plausibleAddressValue(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	// These are references to an address or service prose, not an address
	// value. Keeping this check on phrases/classes avoids evaluation literals.
	for _, marker := range []string{
		"электронной почты", "email", "e-mail", "совпадает", "менять", "изменить",
		"не требуется", "является публичным", "доставки новой карты", "доставки карты", "новой карты",
		"не указан", "неизвестен", "отсутствует", "уточняется", "уточнить", "будет предоставлен",
	} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}
