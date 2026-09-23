package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// birthPlaceLabels — подписи поля «место рождения».
var birthPlaceLabels = []string{"место рождения", "родился в", "родилась в", "уроженец", "уроженка"}

// BirthPlaceDetector находит место рождения в подписанных полях.
type BirthPlaceDetector struct{}

// NewBirthPlaceDetector возвращает детектор места рождения.
func NewBirthPlaceDetector() *BirthPlaceDetector { return &BirthPlaceDetector{} }

// Detect возвращает фрагменты места рождения.
func (d *BirthPlaceDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	lowerText := strings.ToLower(text)
	from := 0
	for {
		labelEnd := findLabel(lowerText, from, birthPlaceLabels)
		if labelEnd < 0 {
			break
		}
		if hasPublicRoleBefore(lowerText, labelEnd) || hasNonPersonalBirthPlaceValue(lowerText, labelEnd) {
			from = labelEnd + 1
			continue
		}
		valStart := skipSeparators(text, labelEnd)
		valStart = skipPersonalFieldQualifier(text, lowerText, valStart)
		valEnd, ok := scanTextValueKeepDot(text, valStart)
		if ok {
			frags = append(frags, Fragment{Type: pii.TypeBirthPlace, Start: valStart, End: valEnd})
			from = valEnd
		} else {
			from = labelEnd + 1
		}
	}
	return frags, nil
}

var publicRoleWords = []string{
	"поэт", "писатель", "художник", "композитор", "учёный", "ученый", "президент",
	"актёр", "актер", "певец", "режиссёр", "режиссер", "космонавт", "политик",
	"министр", "спортсмен", "историк", "философ",
}

func hasPublicRoleBefore(lowerText string, labelEnd int) bool {
	window := windowBefore(lowerText, labelEnd, 160)
	if cut := strings.LastIndexAny(window, ".!?;\n\r"); cut >= 0 {
		window = window[cut+1:]
	}
	for _, role := range publicRoleWords {
		if strings.Contains(window, role) {
			return true
		}
	}
	return false
}

func hasNonPersonalBirthPlaceValue(lowerText string, labelEnd int) bool {
	start := skipSeparators(lowerText, labelEnd)
	for _, prefix := range []string{"компании", "организации", "проекта", "товара"} {
		if strings.HasPrefix(lowerText[start:], prefix) {
			return true
		}
	}
	return false
}
