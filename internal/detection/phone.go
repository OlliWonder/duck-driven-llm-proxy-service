package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// PhoneDetector находит российские номера телефонов в тексте.
//
// Распознаются форматы: +7XXXXXXXXXX, 8XXXXXXXXXX, +7 (XXX) XXX-XX-XX,
// 8-XXX-XXX-XX-XX и т.п. с произвольными разделителями (пробел, дефис,
// скобки). Детектор требует, чтобы номер был ограничен нецифровыми и
// небуквенными символами, поэтому случайные длинные последовательности цифр
// (номера карт, ИНН) не считаются телефонами.
type PhoneDetector struct{}

// NewPhoneDetector возвращает детектор номеров телефонов.
func NewPhoneDetector() *PhoneDetector { return &PhoneDetector{} }

// Detect возвращает фрагменты телефонов с байтовыми смещениями в исходной строке.
func (d *PhoneDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	for i := 0; i < len(text); {
		if hasNonPhoneNumericContext(text, i) {
			i++
			continue
		}
		// Ищем начало номера: '+' или '8' в начале, либо '+' с последующей '7'.
		if text[i] == '+' {
			if i+1 < len(text) && text[i+1] == '7' {
				if end, ok := scanPhone(text, i); ok {
					frags = append(frags, Fragment{Type: pii.TypePhone, Start: i, End: end})
					i = end
					continue
				}
			}
			i++
			continue
		}
		if text[i] == '8' {
			if end, ok := scanPhone(text, i); ok {
				frags = append(frags, Fragment{Type: pii.TypePhone, Start: i, End: end})
				i = end
				continue
			}
		}
		if text[i] == '9' && hasPhoneContext(text, i) {
			if end, ok := scanTenDigitPhone(text, i); ok {
				frags = append(frags, Fragment{Type: pii.TypePhone, Start: i, End: end})
				i = end
				continue
			}
		}
		i++
	}
	return frags, nil
}

// scanPhone пытается распознать номер, начинающийся с позиции start.
// Возвращает конец номера (исключительно) и признак успеха.
func scanPhone(text string, start int) (int, bool) {
	// Проверяем границу слева: перед номером не должно быть цифры или буквы.
	if start > 0 && isWordByte(text[start-1]) {
		return 0, false
	}

	digits := 0
	i := start
	// Первая цифра (8 или 7 после +).
	if text[i] == '+' {
		i++ // '+'
		if i < len(text) && text[i] == '7' {
			i++
			digits++
		} else {
			return 0, false
		}
	} else {
		// '8'
		i++
		digits++
	}

	// Сканируем остальные цифры. Разделители допустимы только между цифрами:
	// если после разделителя нет цифры, номер на этом заканчивается.
	for i < len(text) {
		c := text[i]
		if c >= '0' && c <= '9' {
			digits++
			i++
			continue
		}
		if isSeparator(c) {
			j := i + 1
			for j < len(text) && isSeparator(text[j]) {
				j++
			}
			if j < len(text) && text[j] >= '0' && text[j] <= '9' {
				i = j
				continue
			}
		}
		break
	}

	// Номер должен содержать ровно 11 цифр (код + 10).
	if digits != 11 {
		return 0, false
	}

	// Проверяем границу справа: после номера не должно быть цифры или буквы.
	if i < len(text) && isWordByte(text[i]) {
		return 0, false
	}

	return i, true
}

// isSeparator сообщает, является ли байт допустимым разделителем в номере.
func isSeparator(b byte) bool {
	return b == ' ' || b == '-' || b == '(' || b == ')' || b == '\t'
}

// isWordByte сообщает, является ли байт частью слова (цифра или буква).
func isWordByte(b byte) bool {
	return (b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= 0x80) // кириллица и прочие многобайтовые символы
}

func scanTenDigitPhone(text string, start int) (int, bool) {
	if start > 0 && isWordByte(text[start-1]) {
		return 0, false
	}
	i := start
	for i < len(text) && isDigit(text[i]) && i-start < 10 {
		i++
	}
	if i-start != 10 || (i < len(text) && isWordByte(text[i])) {
		return 0, false
	}
	return i, true
}

func hasPhoneContext(text string, start int) bool {
	lower := strings.ToLower(windowBefore(text, start, 80))
	for _, marker := range []string{"тел", "мобильн", "номер клиента", "phone", "контакт"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func hasNonPhoneNumericContext(text string, start int) bool {
	lower := strings.ToLower(windowBefore(text, start, 32))
	return strings.Contains(lower, "инн") && !hasPhoneContext(text, start)
}
