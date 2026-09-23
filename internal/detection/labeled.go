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
	first, _ := utf8.DecodeRuneInString(label)
	if isWordRune(first) && pos > 0 {
		prev, _ := utf8.DecodeLastRuneInString(text[:pos])
		if isWordRune(prev) {
			return false
		}
	}
	end := pos + len(label)
	last, _ := utf8.DecodeLastRuneInString(label)
	if !isWordRune(last) || end == len(text) {
		return true
	}
	next, _ := utf8.DecodeRuneInString(text[end:])
	return !isWordRune(next)
}

func isWordRune(r rune) bool {
	// UTF-8 punctuation such as «» and emoji is not a word boundary merely
	// because its encoded bytes have the high bit set.
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// skipSeparators пропускает разделители (двоеточие, дефис, пробелы, запятые)
// после подписи. Возвращает позицию первого значимого символа.
func skipSeparators(text string, from int) int {
	i := from
	for i < len(text) {
		c := text[i]
		if c == ':' || c == '-' || c == '=' || c == ' ' || c == '\t' || c == ',' || c == ';' || c == '"' || c == '\'' {
			i++
			continue
		}
		if strings.HasPrefix(text[i:], "—") || strings.HasPrefix(text[i:], "–") || strings.HasPrefix(text[i:], "№") ||
			strings.HasPrefix(text[i:], "«") || strings.HasPrefix(text[i:], "»") ||
			strings.HasPrefix(text[i:], "“") || strings.HasPrefix(text[i:], "”") || strings.HasPrefix(text[i:], "„") {
			_, size := utf8.DecodeRuneInString(text[i:])
			i += size
			continue
		}
		break
	}
	return i
}

// skipPersonalFieldQualifier skips an optional owner between a field label
// and its value, for example "гражданство клиента — РФ".
func skipPersonalFieldQualifier(text, lowerText string, start int) int {
	for _, qualifier := range []string{
		"клиента", "клиентки", "клиенту",
		"заявителя", "заявительницы", "заявителю",
		"заёмщика", "заемщика", "заёмщицы", "заемщицы", "заёмщику", "заемщику",
		"владельца", "владелицы", "владельцу", "получателя", "получателю", "физлица",
	} {
		end := start + len(qualifier)
		if end <= len(lowerText) && strings.HasPrefix(lowerText[start:], qualifier) &&
			(end == len(lowerText) || !isWordByte(lowerText[end])) {
			return skipSeparators(text, end)
		}
	}
	return start
}

// skipValueIntroducer removes neutral form wording between a label and its
// value. These words describe how the field is filled and are never part of
// the PII value itself (for example, "имя клиента указано как Иван Петров").
func skipValueIntroducer(text, lowerText string, start int) int {
	for _, introducer := range []string{
		"указано как", "указана как", "указан как",
		"записано как", "записана как", "записан как",
		"значится как", "указано", "указана", "указан",
	} {
		end := start + len(introducer)
		if end <= len(lowerText) && strings.HasPrefix(lowerText[start:], introducer) &&
			(end == len(lowerText) || !isWordByte(lowerText[end])) {
			return skipSeparators(text, end)
		}
	}
	return start
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
		if c == ',' || c == ';' || c == '\n' || c == '\r' || (c == '.' && !isAddressAbbreviationDot(text, i)) {
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
		if c == ' ' && startsFollowingPIIField(text[i:]) {
			break
		}
		if c == '\n' || c == '\r' || c == ';' {
			break
		}
		if c == '.' && !isAddressAbbreviationDot(text, i) {
			break
		}
		if c == ',' && (nextPIIField(text, i+1) || nextLabeledField(text, i+1) || nextOrganizationAddressClause(text, i+1)) {
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

func startsFollowingPIIField(text string) bool {
	lower := strings.ToLower(text)
	for _, prefix := range []string{
		" и номер карты", " и банковская карта", " и карта", " и телефон", " и номер телефона",
		" и email", " и e-mail", " и электронная почта", " и инн", " и cvv", " и cvc", " и pin", " и пин",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

var addressAbbreviations = map[string]bool{
	"г": true, "ул": true, "д": true, "кв": true, "корп": true,
	"стр": true, "обл": true, "р-н": true, "пр": true, "пр-т": true,
	"пер": true, "ш": true, "наб": true, "пл": true, "пос": true,
	"с": true, "респ": true,
}

func isAddressAbbreviationDot(text string, dot int) bool {
	start := dot
	for start > 0 {
		c := text[start-1]
		if c == ' ' || c == '\t' || c == ',' || c == ':' || c == ';' || c == '=' ||
			c == '(' || c == ')' || c == '[' || c == ']' || c == '\n' || c == '\r' {
			break
		}
		start--
	}
	return addressAbbreviations[strings.ToLower(text[start:dot])]
}

func nextOrganizationAddressClause(text string, start int) bool {
	for start < len(text) && (text[start] == ' ' || text[start] == '\t') {
		start++
	}
	lower := strings.ToLower(text[start:])
	for _, conjunction := range []string{"а ", "но ", "при этом "} {
		if strings.HasPrefix(lower, conjunction) {
			lower = strings.TrimLeft(lower[len(conjunction):], " \t")
			break
		}
	}
	for _, owner := range []string{
		"отделение банка", "отделение", "филиал", "офис банка", "офис",
		"банк", "организация", "компания", "магазин", "представительство",
	} {
		if strings.HasPrefix(lower, owner) {
			return true
		}
	}
	return false
}

// nextLabeledField распознаёт начало следующего подписанного поля после
// запятой (например, "статус заявки: ..."), не перечисляя служебные слова.
func nextLabeledField(text string, start int) bool {
	for start < len(text) && (text[start] == ' ' || text[start] == '\t') {
		start++
	}
	limit := start + 96
	if limit > len(text) {
		limit = len(text)
	}
	seenLetter := false
	for i := start; i < limit; {
		if text[i] == ':' {
			return seenLetter
		}
		if text[i] == ',' || text[i] == ';' || text[i] == '.' || text[i] == '\n' || text[i] == '\r' {
			return false
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if unicode.IsLetter(r) {
			seenLetter = true
		} else if r != ' ' && r != '\t' && r != '-' {
			return false
		}
		i += size
	}
	return false
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
