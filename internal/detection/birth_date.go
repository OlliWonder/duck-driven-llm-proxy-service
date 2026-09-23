package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// birthDateLabels — подписи поля «дата рождения».
var birthDateLabels = []string{
	"сведения о рождении", "дата рождения", "дату рождения", "birth_date", "д.р.", "др",
	"родился", "родилась", "рождён", "рожден", "рождена", "dob",
}

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
		if hasPublicRoleBefore(lowerText, labelEnd) || hasHistoricalBioContext(lowerText, labelEnd) {
			from = labelEnd + 1
			continue
		}
		valStart := skipSeparators(text, labelEnd)
		// Field labels are often followed by the subject before the actual
		// value: "дата рождения клиента — 15.03.1990".
		valStart = skipPersonalFieldQualifier(text, lowerText, valStart)
		valEnd, ok := scanDate(text, valStart)
		if !ok {
			valEnd, ok = scanTextDate(text, valStart)
		}
		if !ok && hasBirthVerbLabel(lowerText, labelEnd) {
			if dateStart, dateEnd, found := findBirthDateNearby(text, valStart, 120); found {
				frags = append(frags, Fragment{Type: pii.TypeBirthDate, Start: dateStart, End: dateEnd})
				from = dateEnd
				continue
			}
		}
		if ok {
			frags = append(frags, Fragment{Type: pii.TypeBirthDate, Start: valStart, End: valEnd})
			from = valEnd
		} else {
			from = labelEnd + 1
		}
	}
	// В короткой табличной строке дата рождения может быть последним столбцом без подписи.
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			continue
		}
		end, ok := scanDate(text, i)
		if ok && hasDelimitedClientDate(text, lowerText, i, end) {
			frags = append(frags, Fragment{Type: pii.TypeBirthDate, Start: i, End: end})
			i = end - 1
		}
	}
	return frags, nil
}

func hasDelimitedClientDate(text, lowerText string, start, end int) bool {
	if start == 0 || end < len(text) && text[end] != ' ' && text[end] != '\t' && text[end] != '|' && text[end] != '\n' && text[end] != '.' {
		return false
	}
	left := strings.LastIndex(text[:start], "|")
	if left < 0 || strings.TrimSpace(text[left+1:start]) != "" {
		return false
	}
	rowStart := strings.LastIndexAny(text[:left], "\n\r") + 1
	if strings.Count(text[rowStart:left], "|") < 1 {
		return false
	}
	return strings.Contains(lowerText[rowStart:left], "клиент")
}

func hasBirthVerbLabel(lowerText string, labelEnd int) bool {
	before := windowBefore(lowerText, labelEnd, 20)
	return strings.Contains(before, "родился") || strings.Contains(before, "родилась") ||
		strings.Contains(before, "рождён") || strings.Contains(before, "рожден") || strings.Contains(before, "рождена")
}

func findBirthDateNearby(text string, start, maxBytes int) (int, int, bool) {
	limit := start + maxBytes
	if limit > len(text) {
		limit = len(text)
	}
	for i := start; i < limit; i++ {
		if text[i] == ';' || text[i] == '\n' || text[i] == '\r' || text[i] == '!' || text[i] == '?' {
			return 0, 0, false
		}
		if text[i] < '0' || text[i] > '9' {
			continue
		}
		if end, ok := scanDate(text, i); ok {
			return i, end, true
		}
	}
	return 0, 0, false
}
