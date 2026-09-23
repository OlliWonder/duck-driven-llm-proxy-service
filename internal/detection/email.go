package detection

import (
	"context"
	"regexp"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// emailRe распознаёт адреса электронной почты. Компилируется один раз при
// инициализации пакета, а не на каждый запрос.
var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z0-9\-]{2,63}`)

// EmailDetector находит адреса электронной почты в тексте.
type EmailDetector struct{}

// NewEmailDetector возвращает детектор адресов электронной почты.
func NewEmailDetector() *EmailDetector { return &EmailDetector{} }

// Detect возвращает фрагменты email с байтовыми смещениями в исходной строке.
func (d *EmailDetector) Detect(_ context.Context, text string) ([]Fragment, error) {
	locs := emailRe.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return nil, nil
	}
	frags := make([]Fragment, 0, len(locs))
	for _, loc := range locs {
		if validEmailCandidate(text, loc[0], loc[1]) {
			frags = append(frags, Fragment{Type: pii.TypeEmail, Start: loc[0], End: loc[1]})
		}
	}
	return frags, nil
}

func validEmailCandidate(text string, start, end int) bool {
	if start > 0 && (isWordByte(text[start-1]) || text[start-1] == '.') {
		return false
	}
	if end < len(text) && (isWordByte(text[end]) || text[end] == '.') {
		return false
	}
	parts := strings.Split(text[start:end], "@")
	if len(parts) != 2 || parts[0] == "" || len(parts[0]) > 64 || strings.HasPrefix(parts[0], ".") ||
		strings.HasSuffix(parts[0], ".") || strings.Contains(parts[0], "..") {
		return false
	}
	domainLabels := strings.Split(parts[1], ".")
	if len(domainLabels) < 2 {
		return false
	}
	for _, label := range domainLabels {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
	}
	return true
}
