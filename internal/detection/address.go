package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// addressLabels — подписи поля «адрес».
var addressLabels = []string{
	"почтовый адрес физлица", "адрес доставки клиента", "клиент живёт по адресу", "клиент живет по адресу",
	"проживает по адресу", "адрес регистрации", "адрес проживания", "адрес клиента",
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
		valEnd, ok := scanAddressValue(text, valStart)
		if ok {
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
