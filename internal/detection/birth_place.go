package detection

import (
	"context"
	"strconv"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// birthPlaceLabels — подписи поля «место рождения».
var birthPlaceLabels = []string{"сведения о рождении", "место рождения", "родился в", "родилась в", "уроженец", "уроженка"}

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
		if hasPublicRoleBefore(lowerText, labelEnd) || hasHistoricalBioContext(lowerText, labelEnd) ||
			hasPublicBioLinkedToClient(lowerText, labelEnd) || hasNonPersonalBirthPlaceValue(lowerText, labelEnd) {
			from = labelEnd + 1
			continue
		}
		valStart := skipSeparators(text, labelEnd)
		valStart = skipPersonalFieldQualifier(text, lowerText, valStart)
		if _, isDate := scanDate(text, valStart); isDate {
			from = labelEnd + 1
			continue
		}
		if _, isDate := scanTextDate(text, valStart); isDate {
			from = labelEnd + 1
			continue
		}
		// A birthplace may itself contain a comma-separated hierarchy, e.g.
		// "Республика Татарстан, г. Казань". Use the address-style scanner,
		// which still stops at a new labelled field or sentence boundary.
		valEnd, ok := scanAddressValue(text, valStart)
		if ok {
			frags = append(frags, Fragment{Type: pii.TypeBirthPlace, Start: valStart, End: valEnd})
			from = valEnd
		} else {
			from = labelEnd + 1
		}
	}
	return frags, nil
}

// hasPublicBioLinkedToClient распознаёт упоминание из публичной биографии,
// если то же имя позже явно указано как имя клиента. Решение относится только
// к этому упоминанию; более позднее поле клиента остаётся персональным.
func hasPublicBioLinkedToClient(lowerText string, labelEnd int) bool {
	before := lowerText[:labelEnd]
	birth := strings.LastIndex(before, "родился")
	if birth < 0 {
		birth = strings.LastIndex(before, "родилась")
	}
	if birth < 0 {
		return false
	}
	subject := strings.TrimSpace(before[:birth])
	for _, marker := range []string{"клиент", "заявител", "пользователь", "сотрудник", "меня зовут", "мой "} {
		if strings.Contains(subject, marker) {
			return false
		}
	}
	words := strings.FieldsFunc(subject, func(r rune) bool {
		return !(r >= 'а' && r <= 'я' || r >= 'А' && r <= 'Я' || r == 'ё' || r == 'Ё' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
	})
	if len(words) < 2 {
		return false
	}
	nameA, nameB := strings.ToLower(words[len(words)-2]), strings.ToLower(words[len(words)-1])
	if len(nameA) < 2 || len(nameB) < 2 {
		return false
	}
	after := lowerText[labelEnd:]
	clientAt := strings.Index(after, "фио клиента")
	if clientAt < 0 {
		clientAt = strings.Index(after, "имя клиента")
	}
	if clientAt < 0 {
		return false
	}
	clientField := after[clientAt:]
	if len(clientField) > 180 {
		clientField = clientField[:180]
	}
	return strings.Contains(clientField, nameA) && strings.Contains(clientField, nameB)
}

func hasHistoricalBioContext(lowerText string, labelEnd int) bool {
	window := windowBefore(lowerText, labelEnd, 220)
	if strings.Contains(window, "историческ") || strings.Contains(window, "биограф") {
		return true
	}
	// Явно исторический год рождения отличает публичную биографию от анкеты
	// нынешнего пользователя без списка имён.
	for i := 0; i+4 <= len(window); i++ {
		year := window[i : i+4]
		if year[0] != '1' || year[1] < '0' || year[1] > '8' {
			continue
		}
		if n, err := strconv.Atoi(year); err == nil && n < 1900 {
			return true
		}
	}
	return false
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
