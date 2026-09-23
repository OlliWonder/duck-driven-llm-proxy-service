package detection

import (
	"context"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// CardNumberDetector находит номера платёжных карт в тексте.
//
// Распознаются последовательности из 13–19 цифр (с необязательными
// разделителями-пробелами/дефисами между группами). Номер принимается, если:
//   - он проходит проверку Luhn.
//
// Это исключает случайные длинные числа, не являющиеся номерами карт.
type CardNumberDetector struct{}

// NewCardNumberDetector возвращает детектор номеров карт.
func NewCardNumberDetector() *CardNumberDetector { return &CardNumberDetector{} }

// Detect возвращает фрагменты номеров карт с байтовыми смещениями.
func (d *CardNumberDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	for i := 0; i < len(text); {
		if !isDigit(text[i]) {
			i++
			continue
		}
		// Собираем цифры, допуская пробелы/дефисы между ними.
		digits := 0
		j := i
		for j < len(text) {
			c := text[j]
			if isDigit(c) {
				digits++
				j++
				continue
			}
			if c == ' ' || c == '-' {
				// Разделитель допустим только если дальше есть цифра.
				k := j + 1
				for k < len(text) && (text[k] == ' ' || text[k] == '-') {
					k++
				}
				if k < len(text) && isDigit(text[k]) {
					j = k
					continue
				}
			}
			break
		}
		if digits >= 13 && digits <= 19 {
			// Границы слова.
			if (i == 0 || !isWordByte(text[i-1])) && (j == len(text) || !isWordByte(text[j])) {
				digitsStr := digitsOnly(text[i:j])
				if luhnValid(digitsStr) && !isExplicitExampleContext(text, i, j) {
					frags = append(frags, Fragment{Type: pii.TypeCardNumber, Start: i, End: j})
				}
			}
		}
		i = j
	}
	return frags, nil
}

// digitsOnly возвращает только цифры из строки.
func digitsOnly(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		if isDigit(s[i]) {
			b = append(b, s[i])
		}
	}
	return string(b)
}

// luhnValid проверяет номер карты по алгоритму Луна.
func luhnValid(s string) bool {
	sum := 0
	double := false
	for i := len(s) - 1; i >= 0; i-- {
		d := int(s[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}
