package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// DrivingLicenseDetector находит серию и номер водительского удостоверения РФ.
//
// Формат: 4 цифры (серия) + 6 цифр (номер), разделённые пробелом или дефисом,
// например "7712 345678". Чтобы не путать с паспортом (тот же формат),
// детектор требует рядом контекстное слово "водительское", "удостоверение"
// или "ВУ".
type DrivingLicenseDetector struct{}

// NewDrivingLicenseDetector возвращает детектор водительского удостоверения.
func NewDrivingLicenseDetector() *DrivingLicenseDetector { return &DrivingLicenseDetector{} }

// Detect возвращает фрагменты водительского удостоверения.
func (d *DrivingLicenseDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	for i := 0; i < len(text); {
		if !isDigit(text[i]) {
			i++
			continue
		}
		if end, ok := scanSeriesNumber(text, i); ok {
			if hasDrivingLicenseContext(text, i) {
				frags = append(frags, Fragment{Type: pii.TypeDrivingLicense, Start: i, End: end})
			}
			i = end
			continue
		}
		i++
	}
	return frags, nil
}

// hasDrivingLicenseContext проверяет наличие контекстного слова рядом с числом.
func hasDrivingLicenseContext(text string, start int) bool {
	window := contactClauseBefore(text, start, 96)
	lower := strings.ToLower(window)
	for _, nonDocument := range []string{"права доступа", "авторские права", "права пользователя"} {
		if strings.Contains(lower, nonDocument) {
			return false
		}
	}
	drivingPos := maxLastIndex(lower, "водительск", "водитель", "удостоверение водителя", "права", "driver license", "в/у", " ву", "ву:", "ву ")
	if pos := lastBoundedIndex(lower, "ву"); pos > drivingPos {
		drivingPos = pos
	}
	passportPos := maxLastIndex(lower, "паспорт", "серия")
	return drivingPos >= 0 && drivingPos > passportPos
}
