package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// fullNameLabels — подписи поля ФИО.
var fullNameLabels = []string{
	"имя клиента", "фамилия клиента", "получатель платежа", "меня зовут",
	"ф.и.о.", "фио", "имя", "фамилия",
}

// FullNameDetector находит ФИО в подписанных полях.
//
// Значение — последовательность из 2–3 слов с заглавной буквы (кириллица или
// латиница), следующая за подписью "фио"/"ф.и.о."/"имя"/"фамилия". В Fragment
// попадает только само значение, без подписи.
type FullNameDetector struct{}

// NewFullNameDetector возвращает детектор ФИО.
func NewFullNameDetector() *FullNameDetector { return &FullNameDetector{} }

// Detect возвращает фрагменты ФИО с байтовыми смещениями.
func (d *FullNameDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	lowerText := strings.ToLower(text)
	from := 0
	for {
		labelEnd := findLabel(lowerText, from, fullNameLabels)
		if labelEnd < 0 {
			break
		}
		valStart := fullNameValueStart(text, labelEnd)
		// A role or card owner between the generic "имя" label and the
		// delimiter belongs to another entity/type, not to the client name.
		if hasNonPersonalNameFieldOwner(lowerText[labelEnd:valStart]) {
			from = labelEnd + 1
			continue
		}
		valEnd, ok := scanFullName(text, valStart)
		if ok {
			frags = append(frags, Fragment{Type: pii.TypeFullName, Start: valStart, End: valEnd})
			from = valEnd
		} else {
			from = labelEnd + 1
		}
	}
	return frags, nil
}

func hasNonPersonalNameFieldOwner(owner string) bool {
	for _, marker := range []string{
		"держател", "владелец карты", "владельца карты",
		"писател", "поэт", "актёр", "актер", "певец", "режисс", "художник", "композитор",
	} {
		if strings.Contains(owner, marker) {
			return true
		}
	}
	return false
}

// scanFullName извлекает 1–4 слова (в любом регистре), начиная с start.
// Подписанное поле ФИО распознаётся независимо от регистра значения.
func scanFullName(text string, start int) (int, bool) {
	return scanNameWords(text, start, 1, 4)
}

// fullNameValueStart поддерживает квалифицированные подписи вида
// "ФИО заявителя: ..." без включения описания владельца поля в значение.
// Правило основано на структуре подписанного поля, а не на списке ролей.
func fullNameValueStart(text string, labelEnd int) int {
	valueStart := skipSeparators(text, labelEnd)
	if hasNameFieldDelimiter(text[labelEnd:valueStart]) {
		return valueStart
	}

	limit := valueStart + 96
	if limit > len(text) {
		limit = len(text)
	}
	for i := valueStart; i < limit; i++ {
		switch text[i] {
		case ':', '=':
			return skipSeparators(text, i+1)
		case ',', ';', '.', '\n', '\r':
			return valueStart
		case '-':
			if i > valueStart && text[i-1] == ' ' && i+1 < len(text) && text[i+1] == ' ' {
				return skipSeparators(text, i+1)
			}
		}
		if strings.HasPrefix(text[i:], "—") || strings.HasPrefix(text[i:], "–") {
			return skipSeparators(text, i+len("—"))
		}
	}
	return valueStart
}

func hasNameFieldDelimiter(text string) bool {
	return strings.ContainsAny(text, ":=-") || strings.Contains(text, "—") || strings.Contains(text, "–")
}
