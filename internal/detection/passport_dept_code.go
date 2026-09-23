package detection

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// PassportDeptCodeDetector находит код подразделения, выдавшего паспорт.
//
// Формат: 3 цифры + дефис + 3 цифры, например "770-123". Требуется контекст
// "код подразделения" или "подразделение", чтобы не путать с другими числами.
type PassportDeptCodeDetector struct{}

// NewPassportDeptCodeDetector возвращает детектор кода подразделения.
func NewPassportDeptCodeDetector() *PassportDeptCodeDetector { return &PassportDeptCodeDetector{} }

// Detect возвращает фрагменты кода подразделения.
func (d *PassportDeptCodeDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	for i := 0; i < len(text); {
		if !isDigit(text[i]) {
			i++
			continue
		}
		if end, ok := scanDeptCode(text, i); ok {
			if hasDeptCodeContext(text, i) {
				frags = append(frags, Fragment{Type: pii.TypePassportDeptCode, Start: i, End: end})
			}
			i = end
			continue
		}
		i++
	}
	return frags, nil
}

// scanDeptCode распознаёт формат "3 цифры - 3 цифры", начиная с start.
func scanDeptCode(text string, start int) (int, bool) {
	if start > 0 && isWordByte(text[start-1]) {
		return 0, false
	}
	i := start
	for k := 0; k < 3; k++ {
		if i >= len(text) || !isDigit(text[i]) {
			return 0, false
		}
		i++
	}
	separatorStart := i
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	if i < len(text) && text[i] == '-' {
		i++
		for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
			i++
		}
	} else if strings.HasPrefix(text[i:], "–") || strings.HasPrefix(text[i:], "—") {
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
		for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
			i++
		}
	} else if i == separatorStart {
		return 0, false
	}
	for k := 0; k < 3; k++ {
		if i >= len(text) || !isDigit(text[i]) {
			return 0, false
		}
		i++
	}
	if i < len(text) && (isWordByte(text[i]) || (text[i] == '-' && i+1 < len(text) && isDigit(text[i+1]))) {
		return 0, false
	}
	return i, true
}

// hasDeptCodeContext проверяет наличие слова "подразделение" рядом.
func hasDeptCodeContext(text string, start int) bool {
	window := contactClauseBefore(text, start, 120)
	lower := strings.ToLower(window)
	return strings.Contains(lower, "подразделен") ||
		strings.Contains(lower, "код органа выдачи") ||
		strings.Contains(lower, "department code") ||
		(strings.Contains(lower, "паспорт") && strings.Contains(lower, "код"))
}
