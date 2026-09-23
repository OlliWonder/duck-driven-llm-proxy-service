package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// citizenshipLabels — подписи поля «гражданство».
var citizenshipLabels = []string{"гражданство", "гражданин", "гражданка", "подданство", "nationality"}

// CitizenshipDetector находит гражданство в подписанных полях.
type CitizenshipDetector struct{}

// NewCitizenshipDetector возвращает детектор гражданства.
func NewCitizenshipDetector() *CitizenshipDetector { return &CitizenshipDetector{} }

// Detect возвращает фрагменты гражданства.
func (d *CitizenshipDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	lowerText := strings.ToLower(text)
	from := 0
	for {
		labelEnd := findLabel(lowerText, from, citizenshipLabels)
		if labelEnd < 0 {
			break
		}
		valStart := skipSeparators(text, labelEnd)
		valStart = skipPersonalFieldQualifier(text, lowerText, valStart)
		valEnd, ok := scanTextValue(text, valStart)
		if ok && plausibleCitizenship(text[valStart:valEnd]) {
			frags = append(frags, Fragment{Type: pii.TypeCitizenship, Start: valStart, End: valEnd})
			from = valEnd
		} else {
			from = labelEnd + 1
		}
	}
	return frags, nil
}

func plausibleCitizenship(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, prefix := range []string{
		"обратил", "приш", "сообщ", "мира", "компани", "организаци", "должен", "может", "не ",
	} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	return len(strings.Fields(lower)) <= 4
}
