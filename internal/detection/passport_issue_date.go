package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// passportIssueDateLabels — подписи поля «дата выдачи паспорта».
var passportIssueDateLabels = []string{"дата выдачи", "выдано", "выдан"}

// PassportIssueDateDetector находит дату выдачи паспорта.
type PassportIssueDateDetector struct{}

// NewPassportIssueDateDetector возвращает детектор даты выдачи паспорта.
func NewPassportIssueDateDetector() *PassportIssueDateDetector { return &PassportIssueDateDetector{} }

// Detect возвращает фрагменты даты выдачи паспорта.
func (d *PassportIssueDateDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	lowerText := strings.ToLower(text)
	from := 0
	for {
		labelEnd := findLabel(lowerText, from, passportIssueDateLabels)
		if labelEnd < 0 {
			break
		}
		if hasNonDocumentIssueContext(lowerText, labelEnd) {
			from = labelEnd + 1
			continue
		}
		valStart := skipSeparators(text, labelEnd)
		valEnd, ok := scanDate(text, valStart)
		if !ok {
			valEnd, ok = scanTextDate(text, valStart)
		}
		if ok {
			frags = append(frags, Fragment{Type: pii.TypePassportIssueDate, Start: valStart, End: valEnd})
			from = valEnd
		} else {
			from = labelEnd + 1
		}
	}
	return frags, nil
}

func hasNonDocumentIssueContext(lowerText string, labelEnd int) bool {
	window := windowBefore(lowerText, labelEnd, 80)
	for _, word := range []string{"товар", "заказ", "задани", "инвентарь", "приз"} {
		if strings.Contains(window, word) {
			return true
		}
	}
	return false
}
