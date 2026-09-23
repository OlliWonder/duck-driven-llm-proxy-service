package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// addressLabels — подписи поля «адрес».
var addressLabels = []string{
	"адрес регистрации и проживания клиента", "адрес регистрации и проживания", "фактический адрес проживания клиента", "фактическое место проживания", "место регистрации заявителя", "место регистрации клиента", "адрес регистрации заявителя", "адрес заявителя", "доставка клиенту по адресу", "адрес проживания клиента", "новый адрес доставки", "почтовый адрес физлица", "адрес доставки клиента", "адрес доставки", "клиент живёт по адресу", "клиент живет по адресу", "клиент живёт в", "клиент живет в",
	"проживает по адресу", "проживает в", "адрес регистрации", "адрес проживания", "адрес клиента",
	"зарегистрирована", "зарегистрирован", "проживает", `"home"`, "домашний адрес", "адрес",
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
		if isNegatedAddressContext(lowerText, labelEnd) {
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
		if email := emailRe.FindStringIndex(text[valStart:]); email != nil && email[0] == 0 {
			from = email[1] + valStart
			continue
		}
		if valStart > 0 && startsFollowingPIIField(text[valStart-1:]) {
			from = labelEnd + 1
			continue
		}
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
	"музея", "музей", "головного офиса", "головной офис", "центрального офиса",
	"публичной площадки", "площадки",
	"отделения банка", "отделения", "отделение банка", "отделение",
	"офиса", "офис", "компании", "организации", "банка", "филиала",
	"филиал", "магазина", "фирмы", "представительства",
	"сайта", "сервера", "приложения",
	"редакции", "издательства",
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
	if strings.Contains(before, "компания зарегистрирована") || strings.Contains(before, "организация зарегистрирована") {
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

// isNegatedAddressContext применяет отрицание только к непосредственно
// указанному адресу и не подавляет следующий личный адрес в том же запросе.
func isNegatedAddressContext(lowerText string, labelEnd int) bool {
	before := strings.TrimSpace(windowBefore(lowerText, labelEnd, 48))
	for _, suffix := range []string{"не адрес клиента", "не адрес проживания", "не адрес регистрации"} {
		if strings.HasSuffix(before, suffix) {
			return true
		}
	}
	for _, suffix := range []string{"не", "не является", "не был", "не была"} {
		if strings.HasSuffix(before, suffix) {
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
