package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// passportIssueDateLabels — подписи поля «дата выдачи паспорта».
var passportIssueDateLabels = []string{"дата оформления паспорта", "дата выдачи паспорта", "дата выдачи документа", "дата выдачи", "выдано", "выдан"}

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
	// In natural passport prose the authority may stand between "выдан" and
	// the date. Recognise that date while keeping the requirement that both
	// passport and issue context are present in the same clause.
	for i := 0; i < len(text); i++ {
		if !isDigit(text[i]) {
			continue
		}
		dateEnd, ok := scanDate(text, i)
		if !ok || !hasPassportIssueDateContext(text, i) {
			continue
		}
		frags = append(frags, Fragment{Type: pii.TypePassportIssueDate, Start: i, End: dateEnd})
		i = dateEnd - 1
	}
	return Merge(frags), nil
}

func hasPassportIssueDateContext(text string, dateStart int) bool {
	before := strings.ToLower(contactClauseBefore(text, dateStart, 256))
	return strings.Contains(before, "паспорт") && strings.Contains(before, "выдан")
}

func hasNonDocumentIssueContext(lowerText string, labelEnd int) bool {
	window := contactClauseBefore(lowerText, labelEnd, 80)
	for _, word := range []string{"товар", "заказ", "задани", "инвентарь", "приз"} {
		if strings.Contains(window, word) {
			return true
		}
	}
	return false
}
