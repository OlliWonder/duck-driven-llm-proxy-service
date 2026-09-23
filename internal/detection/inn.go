package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// INNDetector находит ИНН (10 или 12 цифр) в тексте.
//
// Распознаются только последовательности из 10 или 12 цифр, ограниченные
// нецифровыми символами, с корректной контрольной суммой. Это исключает
// случайные длинные числа (номера карт, телефоны).
type INNDetector struct{}

// NewINNDetector возвращает детектор ИНН.
func NewINNDetector() *INNDetector { return &INNDetector{} }

// Detect возвращает фрагменты ИНН с байтовыми смещениями.
func (d *INNDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	for i := 0; i < len(text); {
		if !isDigit(text[i]) {
			i++
			continue
		}
		// Считаем длину цифровой последовательности.
		j := i
		for j < len(text) && isDigit(text[j]) {
			j++
		}
		n := j - i
		if (n == 10 || n == 12) && validINN(text[i:j]) {
			// Границы слова.
			if (i == 0 || !isWordByte(text[i-1])) && (j == len(text) || !isWordByte(text[j])) && !isOrganizationINNContext(text, i) {
				frags = append(frags, Fragment{Type: pii.TypeINN, Start: i, End: j})
			}
		}
		i = j
	}
	return frags, nil
}

func isOrganizationINNContext(text string, start int) bool {
	before := strings.ToLower(windowBefore(text, start, 192))
	for _, marker := range []string{"инн организации", "инн компании", "инн музея", "инн банка", "инн учреждения", "инн юрлица", "открытом реестре", "публичном реестре"} {
		if strings.Contains(before, marker) {
			return true
		}
	}
	return false
}

// validINN проверяет контрольную сумму ИНН (10 или 12 цифр).
func validINN(s string) bool {
	if len(s) == 10 {
		return innCheck10(s) == int(s[9]-'0')
	}
	if len(s) == 12 {
		return innCheck11(s) == int(s[10]-'0') && innCheck12(s) == int(s[11]-'0')
	}
	return false
}

func innCheck10(s string) int {
	weights := []int{2, 4, 10, 3, 5, 9, 4, 6, 8}
	sum := 0
	for i, w := range weights {
		sum += int(s[i]-'0') * w
	}
	return sum % 11 % 10
}

func innCheck11(s string) int {
	weights := []int{7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
	sum := 0
	for i, w := range weights {
		sum += int(s[i]-'0') * w
	}
	return sum % 11 % 10
}

func innCheck12(s string) int {
	weights := []int{3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
	sum := 0
	for i, w := range weights {
		sum += int(s[i]-'0') * w
	}
	return sum % 11 % 10
}
