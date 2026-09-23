package detection

import (
	"context"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// cardHolderLabels — подписи поля «имя держателя карты».
var cardHolderLabels = []string{
	"на банковской карте указано имя", "на карте указано имя", "имя на банковской карте", "cardholder name", "card_holder", "имя держателя банковской карты", "имя держателя карты", "имя владельца карты", "держатель банковской карты", "держатель карты", "владелец карты", "имя держателя",
	"card holder", "cardholder", "принадлежит", "держатель",
}

// CardHolderDetector находит имя держателя карты в подписанных полях.
type CardHolderDetector struct{}

// NewCardHolderDetector возвращает детектор держателя карты.
func NewCardHolderDetector() *CardHolderDetector { return &CardHolderDetector{} }

// Detect возвращает фрагменты имени держателя карты.
func (d *CardHolderDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	var frags []Fragment
	lowerText := strings.ToLower(text)
	from := 0
	for {
		labelEnd := findLabel(lowerText, from, cardHolderLabels)
		if labelEnd < 0 {
			break
		}
		valStart := skipSeparators(text, labelEnd)
		valEnd, ok := scanCardHolder(text, valStart)
		if ok {
			frags = append(frags, Fragment{Type: pii.TypeCardHolder, Start: valStart, End: valEnd})
			from = valEnd
		} else {
			from = labelEnd + 1
		}
	}
	return frags, nil
}

// scanCardHolder извлекает 1–4 слова (в любом регистре).
func scanCardHolder(text string, start int) (int, bool) {
	return scanNameWords(text, start, 1, 4)
}
