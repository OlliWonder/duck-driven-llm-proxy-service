package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// cvvLabels — подписи поля «CVV-код».
var cvvLabels = []string{"код безопасности карты", "код безопасности", "код cvv", "cvv код", "cvv2", "cvc", "cid карты amex", "cid", "cvv"}

// CVVDetector находит CVV-код карты в подписанных полях.
//
// CVV распознаётся только при явном контексте (подпись "cvv"/"код
// безопасности"), а не по длине числа.
type CVVDetector struct{}

// NewCVVDetector возвращает детектор CVV-кода.
func NewCVVDetector() *CVVDetector { return &CVVDetector{} }

// Detect возвращает фрагменты CVV-кода.
func (d *CVVDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	lowerText := strings.ToLower(text)
	from := 0
	for {
		labelEnd := findLabel(lowerText, from, cvvLabels)
		if labelEnd < 0 {
			break
		}
		valStart := skipSeparators(text, labelEnd)
		valEnd, ok := scanDigits(text, valStart)
		if ok {
			n := valEnd - valStart
			if n == 3 || n == 4 {
				frags = append(frags, Fragment{Type: pii.TypeCVV, Start: valStart, End: valEnd})
				from = valEnd
				continue
			}
		}
		from = labelEnd + 1
	}
	return frags, nil
}
