package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// birthDateLabels — подписи поля «дата рождения».
var birthDateLabels = []string{"дата рождения", "д.р.", "родился", "родилась"}

// BirthDateDetector находит дату рождения в подписанных полях.
type BirthDateDetector struct{}

// NewBirthDateDetector возвращает детектор даты рождения.
func NewBirthDateDetector() *BirthDateDetector { return &BirthDateDetector{} }

// Detect возвращает фрагменты даты рождения.
func (d *BirthDateDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	lowerText := strings.ToLower(text)
	from := 0
	for {
		labelEnd := findLabel(lowerText, from, birthDateLabels)
		if labelEnd < 0 {
			break
		}
		if hasPublicRoleBefore(lowerText, labelEnd) {
			from = labelEnd + 1
			continue
		}
		valStart := skipSeparators(text, labelEnd)
		valEnd, ok := scanDate(text, valStart)
		if !ok {
			valEnd, ok = scanTextDate(text, valStart)
		}
		if ok {
			frags = append(frags, Fragment{Type: pii.TypeBirthDate, Start: valStart, End: valEnd})
			from = valEnd
		} else {
			from = labelEnd + 1
		}
	}
	return frags, nil
}
