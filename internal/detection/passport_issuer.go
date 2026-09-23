package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// passportIssuerLabels — подписи поля «орган, выдавший паспорт».
var passportIssuerLabels = []string{
	"орган, выдавший документ", "орган, выдавший паспорт", "кем выдан паспорт", "орган выдачи паспорта", "паспорт выдал", "кем выдан", "орган выдачи",
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

	// Natural passport prose commonly puts the issue date between "выдан"
	// and the authority: "выдан 18.07.2015 Отделом УФМС ...".
	from = 0
	for {
		idx := strings.Index(lowerText[from:], "выдан")
		if idx < 0 {
			break
		}
		wordStart := from + idx
		wordEnd := wordStart + len("выдан")
		from = wordEnd
		if (wordStart > 0 && isWordByte(lowerText[wordStart-1])) ||
			(wordEnd < len(lowerText) && isWordByte(lowerText[wordEnd])) {
			continue
		}
		valStart := skipSeparators(text, wordEnd)
		for _, qualifier := range []string{"паспорт", "документ"} {
			end := valStart + len(qualifier)
			if end <= len(lowerText) && strings.HasPrefix(lowerText[valStart:], qualifier) &&
				(end == len(lowerText) || !isWordByte(lowerText[end])) {
				valStart = skipSeparators(text, end)
				break
			}
		}
		if dateEnd, ok := scanDate(text, valStart); ok {
			valStart = skipSeparators(text, dateEnd)
		}
		valEnd, ok := scanTextValueKeepDot(text, valStart)
		if ok && looksLikePassportIssuer(text[valStart:valEnd]) &&
			(hasPassportIssuerContext(lowerText, wordStart) || looksLikeOfficialPassportAuthority(text[valStart:valEnd])) {
			frags = append(frags, Fragment{Type: pii.TypePassportIssuer, Start: valStart, End: valEnd})
		}
	}
	return Merge(frags), nil
}

func hasPassportIssuerContext(lowerText string, end int) bool {
	clause := contactClauseBefore(lowerText, end, 192)
	return strings.Contains(clause, "паспорт") || strings.Contains(clause, "документ") || strings.Contains(clause, "серия")
}

func looksLikeOfficialPassportAuthority(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"уфмс", "мвд", "овд", "миграц"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikePassportIssuer(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, marker := range []string{"уфмс", "мвд", "отдел", "управлен", "овд", "миграц"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
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
