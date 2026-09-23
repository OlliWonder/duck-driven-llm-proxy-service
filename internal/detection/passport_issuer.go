package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// passportIssuerLabels — подписи поля «орган, выдавший паспорт».
var passportIssuerLabels = []string{
	"орган, выдавший документ", "паспорт выдал", "кем выдан", "орган выдачи",
}

// PassportIssuerDetector находит орган, выдавший паспорт.
type PassportIssuerDetector struct{}

// NewPassportIssuerDetector возвращает детектор органа выдачи.
func NewPassportIssuerDetector() *PassportIssuerDetector { return &PassportIssuerDetector{} }

// Detect возвращает фрагменты органа выдачи.
func (d *PassportIssuerDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	lowerText := strings.ToLower(text)
	from := 0
	for {
		labelEnd := findLabel(lowerText, from, passportIssuerLabels)
		if labelEnd < 0 {
			break
		}
		valStart := skipSeparators(text, labelEnd)
		valEnd, ok := scanTextValueKeepDot(text, valStart)
		if ok && plausiblePassportIssuer(text[valStart:valEnd]) {
			frags = append(frags, Fragment{Type: pii.TypePassportIssuer, Start: valStart, End: valEnd})
			from = valEnd
		} else {
			from = labelEnd + 1
		}
	}
	return frags, nil
}

func plausiblePassportIssuer(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range []string{"новост", "товар", "кредит", "задани", "сообщени"} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	return lower != ""
}
