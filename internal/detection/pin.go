package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// pinLabels — подписи поля «пин-код карты».
var pinLabels = []string{"в поле pin указано", "код pin клиента", "пин-код клиента", "пин код клиента", "pin-код клиента", "pin код клиента", "пин-код карты", "пин код карты", "pin-код карты", "pin код карты", "пин карты", "pin карты", "пин-код", "пин код", "pin-код", "pin код", "пин", "pin"}

// PINDetector находит пин-код карты в подписанных полях.
//
// PIN распознаётся только при явном контексте (подпись "пин"/"pin"),
// а не по длине числа.
type PINDetector struct{}

// NewPINDetector возвращает детектор пин-кода.
func NewPINDetector() *PINDetector { return &PINDetector{} }

// Detect возвращает фрагменты пин-кода.
func (d *PINDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	lowerText := strings.ToLower(text)
	from := 0
	for {
		labelEnd := findLabel(lowerText, from, pinLabels)
		if labelEnd < 0 {
			break
		}
		valStart := skipSeparators(text, labelEnd)
		valEnd, ok := scanDigits(text, valStart)
		if ok {
			n := valEnd - valStart
			if n >= 4 && n <= 6 {
				frags = append(frags, Fragment{Type: pii.TypePIN, Start: valStart, End: valEnd})
				from = valEnd
				continue
			}
		}
		from = labelEnd + 1
	}
	return frags, nil
}
