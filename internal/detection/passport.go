package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// PassportDetector находит серию и номер паспорта РФ в тексте.
//
// Формат: 4 цифры (серия) + 6 цифр (номер), разделённые пробелом или дефисом,
// например "4509 123456". Чтобы не путать с водительским удостоверением
// (тот же формат), детектор требует рядом контекстное слово "паспорт" или
// "серия".
type PassportDetector struct{}

// NewPassportDetector возвращает детектор паспорта РФ.
func NewPassportDetector() *PassportDetector { return &PassportDetector{} }

// Detect возвращает фрагменты паспорта с байтовыми смещениями в исходной строке.
func (d *PassportDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	for i := 0; i < len(text); {
		if !isDigit(text[i]) {
			i++
			continue
		}
		// Пытаемся распознать серию и номер, начиная с i.
		if end, ok := scanSeriesNumber(text, i); ok {
			if hasPassportContext(text, i) {
				frags = append(frags, Fragment{Type: pii.TypePassportSeries, Start: i, End: end})
			}
			i = end
			continue
		}
		i++
	}
	return frags, nil
}

// scanSeriesNumberSplit распознаёт формат «серия XXXX номер YYYYYY», начиная с
// позиции start (первая цифра серии). Возвращает конец номера (исключительно)
// и признак успеха.
func scanSeriesNumberSplit(text string, start int) (int, bool) {
	return scanSeriesNumber(text, start)
}

// scanSeriesNumber распознаёт формат "4 цифры [разделитель] 6 цифр",
// начиная с позиции start. Возвращает конец (исключительно) и признак успеха.
func scanSeriesNumber(text string, start int) (int, bool) {
	if start > 0 && isWordByte(text[start-1]) {
		return 0, false
	}
	// Four series digits, either contiguous or grouped as "45 09".
	i := start
	for k := 0; k < 2; k++ {
		if i >= len(text) || !isDigit(text[i]) {
			return 0, false
		}
		i++
	}
	if i < len(text) && text[i] == ' ' && i+2 < len(text) && isDigit(text[i+1]) && isDigit(text[i+2]) {
		i++
	}
	for k := 0; k < 2; k++ {
		if i >= len(text) || !isDigit(text[i]) {
			return 0, false
		}
		i++
	}

	separatorStart := i
	for i < len(text) && (text[i] == ' ' || text[i] == '-' || text[i] == ',' || text[i] == ':') {
		i++
	}
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower[i:], "номер") && hasLabelBoundaries(lower, i, "номер") {
		i += len("номер")
	} else if strings.HasPrefix(text[i:], "№") {
		i += len("№")
	}
	for i < len(text) && (text[i] == ' ' || text[i] == '-' || text[i] == ':' || text[i] == ',') {
		i++
	}
	if i == separatorStart {
		return 0, false
	}
	numberStart := i
	for i < len(text) && isDigit(text[i]) && i-numberStart < 6 {
		i++
	}
	if i-numberStart != 6 {
		return 0, false
	}
	// Граница справа: не должно быть цифры или буквы.
	if i < len(text) && (isWordByte(text[i]) || (text[i] == '-' && i+1 < len(text) && isDigit(text[i+1]))) {
		return 0, false
	}
	return i, true
}

// hasPassportContext проверяет, есть ли рядом с позицией start слово
// "паспорт" или "серия".
func hasPassportContext(text string, start int) bool {
	// Ищем в окне до 96 байт перед числом.
	window := contactClauseBefore(text, start, 96)
	lower := strings.ToLower(window)
	for _, nonDocument := range []string{"серия книги", "серия товара", "серия модели", "серия выпуска"} {
		if strings.Contains(lower, nonDocument) {
			return false
		}
	}
	passportPos := maxLastIndex(lower, "паспорт", "серия")
	drivingPos := maxLastIndex(lower, "водительск", "удостоверение водителя", "права", "driver license", " ву", "ву:", "ву ")
	return passportPos >= 0 && passportPos > drivingPos
}

func maxLastIndex(text string, markers ...string) int {
	best := -1
	for _, marker := range markers {
		if pos := strings.LastIndex(text, marker); pos > best {
			best = pos
		}
	}
	return best
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
