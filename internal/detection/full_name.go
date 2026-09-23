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
		valStart := skipSeparators(text, labelEnd)
		// "имя держателя" belongs to the more specific card-holder detector.
		if strings.HasPrefix(lowerText[valStart:], "держателя") {
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

// scanFullName извлекает 1–4 слова (в любом регистре), начиная с start.
// Подписанное поле ФИО распознаётся независимо от регистра значения.
func scanFullName(text string, start int) (int, bool) {
	return scanNameWords(text, start, 1, 4)
}
