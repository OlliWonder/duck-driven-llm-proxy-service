package detection

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// labelMatch описывает найденную подпись поля.
type labelMatch struct {
	// labelEnd — байтовая позиция сразу после подписи (до разделителей).
	labelEnd int
}

// findLabel ищет первую подпись из labels в lowerText (текст в нижнем
// регистре), начиная с позиции from. Возвращает позицию сразу после подписи
// или -1.
func findLabel(lowerText string, from int, labels []string) int {
	best := -1
	bestLabel := ""
	for _, l := range labels {
		searchFrom := from
		for searchFrom <= len(lowerText) {
			idx := strings.Index(lowerText[searchFrom:], l)
			if idx < 0 {
				break
			}
			pos := searchFrom + idx
			if hasLabelBoundaries(lowerText, pos, l) {
				if best == -1 || pos < best || (pos == best && len(l) > len(bestLabel)) {
					best = pos
					bestLabel = l
				}
				break
			}
			searchFrom = pos + 1
		}
	}
	if best < 0 {
		return -1
	}
	return best + len(bestLabel)
}

func hasLabelBoundaries(text string, pos int, label string) bool {
	if isWordByte(label[0]) && pos > 0 && isWordByte(text[pos-1]) {
		return false
	}
	end := pos + len(label)
	return !isWordByte(label[len(label)-1]) || end == len(text) || !isWordByte(text[end])
}

// skipSeparators пропускает разделители (двоеточие, дефис, пробелы, запятые)
// после подписи. Возвращает позицию первого значимого символа.
func skipSeparators(text string, from int) int {
	i := from
	for i < len(text) {
		c := text[i]
		if c == ':' || c == '-' || c == '=' || c == ' ' || c == '\t' || c == ',' || c == ';' {
			i++
			continue
		}
		if strings.HasPrefix(text[i:], "—") || strings.HasPrefix(text[i:], "–") || strings.HasPrefix(text[i:], "№") {
			_, size := utf8.DecodeRuneInString(text[i:])
			i += size
			continue
		}
		break
	}
	return i
}

// isCyrillicLetter сообщает, является ли байт частью кириллической буквы
// (первый байт 0xD0–0xD1 или второй байт 0x80–0xBF в UTF-8).
func isCyrillicLetter(b byte) bool {
	return b >= 0x80 && b <= 0xFF
}

// isLatinLetter сообщает, является ли байт латинской буквой.
func isLatinLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// isLetter сообщает, является ли байт буквой (кириллица или латиница).
func isLetter(b byte) bool {
	return isCyrillicLetter(b) || isLatinLetter(b)
}

// isLetterAt сообщает, является ли символ в позиции i буквой (кириллица или
// латиница), в любом регистре.
func isLetterAt(text string, i int) bool {
	if i >= len(text) {
		return false
	}
	return isLetter(text[i])
}

// isUpperAt сообщает, является ли символ в позиции i заглавной буквой
// (кириллица или латиница).
func isUpperAt(text string, i int) bool {
	if i >= len(text) {
		return false
	}
	b := text[i]
	if b >= 'A' && b <= 'Z' {
		return true
	}
	// Кириллица: заглавные буквы А-Я = 0xD0 0x90–0xAF, Ё = 0xD0 0x81.
	if b == 0xD0 && i+1 < len(text) {
		c := text[i+1]
		return (c >= 0x90 && c <= 0xAF) || c == 0x81
	}
	return false
}

// isValueSeparator сообщает, является ли байт разделителем значения.
func isValueSeparator(b byte) bool {
	return b == ',' || b == ';' || b == '.' || b == '\n' || b == '\r'
}

// scanTextValue извлекает текстовое значение, начиная с start, до разделителя
// (запятая, точка с запятой, точка, перевод строки) или до конца строки.
// Обрезает хвостовые пробелы. Возвращает конец значения и признак успеха.
func scanTextValue(text string, start int) (int, bool) {
	if start >= len(text) {
		return 0, false
	}
	i := start
	for i < len(text) && !isValueSeparator(text[i]) {
		i++
	}
	// Обрезаем хвостовые пробелы.
	for i > start && text[i-1] == ' ' {
		i--
	}
	if i == start {
		return 0, false
	}
	return i, true
}

// scanTextValueKeepDot извлекает текстовое значение, но точка не считается
// разделителем (нужно для значений с сокращениями вида "г. Москва").
// Разделители: запятая, точка с запятой, перевод строки.
func scanTextValueKeepDot(text string, start int) (int, bool) {
	if start >= len(text) {
		return 0, false
	}
	i := start
	for i < len(text) {
		c := text[i]
		if c == ',' || c == ';' || c == '\n' || c == '\r' {
			break
		}
		i++
	}
	for i > start && text[i-1] == ' ' {
		i--
	}
	if i == start {
		return 0, false
	}
	return i, true
}

// scanAddressValue извлекает адрес, начиная с start. В отличие от
// scanTextValue, запятые внутри адреса не считаются разделителями — адрес
// заканчивается на перевод строки, точку с запятой или конец строки.
func scanAddressValue(text string, start int) (int, bool) {
	if start >= len(text) {
		return 0, false
	}
	i := start
	for i < len(text) {
		c := text[i]
		if c == '\n' || c == '\r' || c == ';' {
			break
		}
		if c == ',' && nextPIIField(text, i+1) {
			break
		}
		i++
	}
	for i > start && text[i-1] == ' ' {
		i--
	}
	if i == start {
		return 0, false
	}
	return i, true
}

func nextPIIField(text string, start int) bool {
	for start < len(text) && (text[start] == ' ' || text[start] == '\t') {
		start++
	}
	lower := strings.ToLower(text[start:])
	for _, label := range []string{
		"телефон", "тел.", "тел ", "email", "e-mail", "почта", "паспорт", "инн",
		"дата рождения", "гражданство", "кем выдан", "код подразделения", "cvv", "cvc",
		"pin", "пин", "карта", "номер карты", "фио",
	} {
		if strings.HasPrefix(lower, label) {
			return true
		}
	}
	return false
}

func windowBefore(text string, end, maxBytes int) string {
	start := end - maxBytes
	if start < 0 {
		start = 0
	}
	for start < end && text[start]&0xC0 == 0x80 {
		start++
	}
	return text[start:end]
}

func scanNameWords(text string, start, minWords, maxWords int) (int, bool) {
	if start < 0 || start >= len(text) {
		return 0, false
	}
	i := start
	words := 0
	for words < maxWords {
		for i < len(text) && text[i] == ' ' {
			i++
		}
		wordStart := i
		for i < len(text) {
			r, size := utf8.DecodeRuneInString(text[i:])
			if unicode.IsLetter(r) || ((r == '-' || r == '\'') && i > wordStart) {
				i += size
				continue
			}
			break
		}
		if i == wordStart {
			break
		}
		words++
	}
	for i > start && text[i-1] == ' ' {
		i--
	}
	return i, words >= minWords
}

// scanDigits извлекает последовательность цифр, начиная с start.
// Возвращает конец и признак успеха (есть хотя бы одна цифра).
func scanDigits(text string, start int) (int, bool) {
	if start >= len(text) || !isDigit(text[start]) {
		return 0, false
	}
	i := start
	for i < len(text) && isDigit(text[i]) {
		i++
	}
	// Граница справа: не должно быть цифры или буквы.
	if i < len(text) && isWordByte(text[i]) {
		return 0, false
	}
	return i, true
}
