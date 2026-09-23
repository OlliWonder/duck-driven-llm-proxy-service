package detection

import (
	"strconv"
	"strings"
	"time"
)

// scanDate распознаёт дату в числовом формате, начиная с start. Поддерживаются
// форматы ДД.ММ.ГГГГ, ГГГГ-ММ-ДД, ДД/ММ/ГГГГ и ДД-ММ-ГГГГ.
// Возвращает конец даты (исключительно) и признак успеха.
func scanDate(text string, start int) (int, bool) {
	// ДД.ММ.ГГГГ / ДД.ММ.ГГ
	if end, ok := scanDottedDate(text, start); ok {
		return end, true
	}
	// ГГГГ-ММ-ДД
	if end, ok := scanISODate(text, start); ok {
		return end, true
	}
	// ДД-ММ-ГГГГ или ММ-ДД-ГГГГ.
	if end, ok := scanHyphenDate(text, start); ok {
		return end, true
	}
	// ДД/ММ/ГГГГ
	if end, ok := scanSlashDate(text, start); ok {
		return end, true
	}
	return 0, false
}

// scanHyphenDate распознаёт дату с дефисами. Четырёхзначный первый компонент
// остаётся ISO-форматом и обрабатывается scanISODate.
func scanHyphenDate(text string, start int) (int, bool) {
	if start+10 > len(text) || !twoDigitsAt(text, start) || text[start+2] != '-' ||
		!twoDigitsAt(text, start+3) || text[start+5] != '-' || !fourDigitsAt(text, start+6) {
		return 0, false
	}
	end := start + 10
	first, second := numberAt(text, start, 2), numberAt(text, start+3, 2)
	year := numberAt(text, start+6, 4)
	if !validDateBoundary(text, end) ||
		(!validCalendarDate(first, second, year) && !validCalendarDate(second, first, year)) {
		return 0, false
	}
	return end, true
}

// scanDottedDate распознаёт ДД.ММ.ГГГГ или ДД.ММ.ГГ.
func scanDottedDate(text string, start int) (int, bool) {
	// DD.MM.YY / DD.MM.YYYY and MM.DD.YYYY. The span is identical for
	// ambiguous values, so accepting either valid calendar interpretation is
	// sufficient for PII detection.
	if start+8 <= len(text) && twoDigitsAt(text, start) && text[start+2] == '.' &&
		twoDigitsAt(text, start+3) && text[start+5] == '.' && twoDigitsAt(text, start+6) {
		end := start + 8
		year := numberAt(text, start+6, 2) + 2000
		if start+10 <= len(text) && twoDigitsAt(text, start+8) {
			end = start + 10
			year = numberAt(text, start+6, 4)
		}
		first, second := numberAt(text, start, 2), numberAt(text, start+3, 2)
		if validDateBoundary(text, end) && (validCalendarDate(first, second, year) || validCalendarDate(second, first, year)) {
			return end, true
		}
	}
	// YYYY.DD.MM and YYYY.MM.DD.
	if start+10 <= len(text) && fourDigitsAt(text, start) && text[start+4] == '.' &&
		twoDigitsAt(text, start+5) && text[start+7] == '.' && twoDigitsAt(text, start+8) {
		year := numberAt(text, start, 4)
		first, second := numberAt(text, start+5, 2), numberAt(text, start+8, 2)
		end := start + 10
		if validDateBoundary(text, end) && (validCalendarDate(first, second, year) || validCalendarDate(second, first, year)) {
			return end, true
		}
	}
	return 0, false
}

// scanISODate распознаёт ГГГГ-ММ-ДД.
func scanISODate(text string, start int) (int, bool) {
	if start+10 > len(text) {
		return 0, false
	}
	// ГГГГ-
	for k := 0; k < 4; k++ {
		if !isDigit(text[start+k]) {
			return 0, false
		}
	}
	if text[start+4] != '-' {
		return 0, false
	}
	// ММ-
	if !isDigit(text[start+5]) || !isDigit(text[start+6]) || text[start+7] != '-' {
		return 0, false
	}
	// ДД
	if !isDigit(text[start+8]) || !isDigit(text[start+9]) {
		return 0, false
	}
	end := start + 10
	if !validDateBoundary(text, end) || !validCalendarDate(numberAt(text, start+8, 2), numberAt(text, start+5, 2), numberAt(text, start, 4)) {
		return 0, false
	}
	return end, true
}

// scanSlashDate распознаёт ДД/ММ/ГГГГ.
func scanSlashDate(text string, start int) (int, bool) {
	if start+10 > len(text) {
		return 0, false
	}
	if !isDigit(text[start]) || !isDigit(text[start+1]) || text[start+2] != '/' {
		return 0, false
	}
	if !isDigit(text[start+3]) || !isDigit(text[start+4]) || text[start+5] != '/' {
		return 0, false
	}
	if !isDigit(text[start+6]) || !isDigit(text[start+7]) || !isDigit(text[start+8]) || !isDigit(text[start+9]) {
		return 0, false
	}
	end := start + 10
	first, second := numberAt(text, start, 2), numberAt(text, start+3, 2)
	if !validDateBoundary(text, end) ||
		(!validCalendarDate(first, second, numberAt(text, start+6, 4)) && !validCalendarDate(second, first, numberAt(text, start+6, 4))) {
		return 0, false
	}
	return end, true
}

func twoDigitsAt(text string, start int) bool {
	return start+2 <= len(text) && isDigit(text[start]) && isDigit(text[start+1])
}

func fourDigitsAt(text string, start int) bool {
	return start+4 <= len(text) && twoDigitsAt(text, start) && twoDigitsAt(text, start+2)
}

func numberAt(text string, start, length int) int {
	n, _ := strconv.Atoi(text[start : start+length])
	return n
}

func validDateBoundary(text string, end int) bool {
	return end == len(text) || !isWordByte(text[end])
}

func validCalendarDate(day, month, year int) bool {
	if year < 1900 || year > 2100 || month < 1 || month > 12 || day < 1 || day > 31 {
		return false
	}
	parsed := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return parsed.Year() == year && int(parsed.Month()) == month && parsed.Day() == day
}

// russianMonths — названия месяцев в именительном и родительном падежах.
var russianMonths = []string{
	"января", "январь",
	"февраля", "февраль",
	"марта", "март",
	"апреля", "апрель",
	"мая", "май",
	"июня", "июнь",
	"июля", "июль",
	"августа", "август",
	"сентября", "сентябрь",
	"октября", "октябрь",
	"ноября", "ноябрь",
	"декабря", "декабрь",
}

// russianDayWords — слова-дни месяца (родительный и именительный падежи).
var russianDayWords = []string{
	"первого", "первое",
	"второго", "второе",
	"третьего", "третье",
	"четвертого", "четвёртого", "четвертое", "четвёртое",
	"пятого", "пятое",
	"шестого", "шестое",
	"седьмого", "седьмое",
	"восьмого", "восьмое",
	"девятого", "девятое",
	"десятого", "десятое",
	"одиннадцатого", "одиннадцатое",
	"двенадцатого", "двенадцатое",
	"тринадцатого", "тринадцатое",
	"четырнадцатого", "четырнадцатое",
	"пятнадцатого", "пятнадцатое",
	"шестнадцатого", "шестнадцатое",
	"семнадцатого", "семнадцатое",
	"восемнадцатого", "восемнадцатое",
	"девятнадцатого", "девятнадцатое",
	"двадцатого", "двадцатое",
	"двадцать первого", "двадцать первое",
	"двадцать второго", "двадцать второе",
	"двадцать третьего", "двадцать третье",
	"двадцать четвертого", "двадцать четвёртого", "двадцать четвертое", "двадцать четвёртое",
	"двадцать пятого", "двадцать пятое",
	"двадцать шестого", "двадцать шестое",
	"двадцать седьмого", "двадцать седьмое",
	"двадцать восьмого", "двадцать восьмое",
	"двадцать девятого", "двадцать девятое",
	"тридцатого", "тридцатое",
	"тридцать первого", "тридцать первое",
}

// scanTextDate распознаёт дату текстом вида «пятнадцатое марта 1990 года»,
// начиная с start. Возвращает конец даты (исключительно) и признак успеха.
func scanTextDate(text string, start int) (int, bool) {
	lower := strings.ToLower(text)
	if end, ok := scanNumericTextDate(text, lower, start); ok {
		return end, true
	}
	// Ищем день словами.
	dayEnd := -1
	for _, d := range russianDayWords {
		if strings.HasPrefix(lower[start:], d) {
			dayEnd = start + len(d)
			break
		}
	}
	if dayEnd < 0 {
		return 0, false
	}
	// Пропускаем пробелы.
	i := dayEnd
	for i < len(text) && text[i] == ' ' {
		i++
	}
	// Ищем месяц.
	monthEnd := -1
	for _, m := range russianMonths {
		if strings.HasPrefix(lower[i:], m) {
			monthEnd = i + len(m)
			break
		}
	}
	if monthEnd < 0 {
		return 0, false
	}
	// Пропускаем пробелы.
	j := monthEnd
	for j < len(text) && text[j] == ' ' {
		j++
	}
	if end, ok := scanRussianYearWords(lower, j); ok {
		return end, true
	}
	// Ищем год (4 цифры).
	if j+4 > len(text) || !isDigit(text[j]) || !isDigit(text[j+1]) || !isDigit(text[j+2]) || !isDigit(text[j+3]) {
		return 0, false
	}
	end := j + 4
	// Необязательное слово «года»/«год».
	k := end
	for k < len(text) && text[k] == ' ' {
		k++
	}
	if strings.HasPrefix(lower[k:], "года") {
		end = k + len("года")
	} else if strings.HasPrefix(lower[k:], "год") {
		end = k + len("год")
	}
	if end < len(text) && isWordByte(text[end]) {
		return 0, false
	}
	return end, true
}

func scanRussianYearWords(lower string, start int) (int, bool) {
	const prefix = "две тысячи "
	if !strings.HasPrefix(lower[start:], prefix) {
		return 0, false
	}
	yearStart := start + len(prefix)
	for _, suffix := range russianYearSuffixes {
		if !strings.HasPrefix(lower[yearStart:], suffix) {
			continue
		}
		end := yearStart + len(suffix)
		if end < len(lower) && isWordByte(lower[end]) {
			continue
		}
		for end < len(lower) && lower[end] == ' ' {
			end++
		}
		if strings.HasPrefix(lower[end:], "года") {
			end += len("года")
		} else if strings.HasPrefix(lower[end:], "год") {
			end += len("год")
		}
		if end < len(lower) && isWordByte(lower[end]) {
			return 0, false
		}
		return end, true
	}
	return 0, false
}

var russianYearSuffixes = []string{
	"первого", "второго", "третьего", "четвертого", "четвёртого",
	"пятого", "шестого", "седьмого", "восьмого", "девятого",
	"десятого", "одиннадцатого", "двенадцатого", "тринадцатого",
	"четырнадцатого", "пятнадцатого", "шестнадцатого", "семнадцатого",
	"восемнадцатого", "девятнадцатого", "двадцатого",
	"двадцать первого", "двадцать второго", "двадцать третьего",
	"двадцать четвертого", "двадцать четвёртого", "двадцать пятого",
	"двадцать шестого", "двадцать седьмого", "двадцать восьмого",
	"двадцать девятого", "тридцатого", "тридцать первого",
}

// scanNumericTextDate recognises dates such as "7 мая 1988 года".
func scanNumericTextDate(text, lower string, start int) (int, bool) {
	i := start
	for i < len(text) && isDigit(text[i]) && i-start < 2 {
		i++
	}
	if i == start || (i < len(text) && isDigit(text[i])) {
		return 0, false
	}
	day := numberAt(text, start, i-start)
	for i < len(text) && text[i] == ' ' {
		i++
	}
	month := 0
	monthEnd := 0
	for index, name := range russianMonths {
		if strings.HasPrefix(lower[i:], name) && len(name) > monthEnd-i {
			month = index/2 + 1
			monthEnd = i + len(name)
		}
	}
	if month == 0 {
		return 0, false
	}
	i = monthEnd
	for i < len(text) && text[i] == ' ' {
		i++
	}
	if !fourDigitsAt(text, i) {
		return 0, false
	}
	year := numberAt(text, i, 4)
	if !validCalendarDate(day, month, year) {
		return 0, false
	}
	end := i + 4
	k := end
	for k < len(text) && text[k] == ' ' {
		k++
	}
	if strings.HasPrefix(lower[k:], "года") {
		end = k + len("года")
	} else if strings.HasPrefix(lower[k:], "год") {
		end = k + len("год")
	}
	if !validDateBoundary(text, end) {
		return 0, false
	}
	return end, true
}
