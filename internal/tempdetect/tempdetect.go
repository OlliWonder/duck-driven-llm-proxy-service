// Пакет tempdetect предоставляет временный правиловой детектор, чтобы ядро
// сервиса могло работать сквозным образом до интеграции реального детектора
// участника 2. Он реализует контракт detection.Detector.
//
// ВРЕМЕННО: замените на реальный детектор из internal/detection.
package tempdetect

import (
	"context"
	"regexp"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

var (
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phoneRe = regexp.MustCompile(`(?:\+7|8)[\s\-]?\(?\d{3}\)?[\s\-]?\d{3}[\s\-]?\d{2}[\s\-]?\d{2}`)
)

// Detector — временный правиловой детектор для email и телефона.
type Detector struct{}

// New возвращает временный детектор.
func New() *Detector { return &Detector{} }

// Detect реализует detection.Detector.
func (d *Detector) Detect(_ context.Context, text string) ([]detection.Fragment, error) {
	var frags []detection.Fragment
	for _, loc := range emailRe.FindAllStringIndex(text, -1) {
		frags = append(frags, detection.Fragment{Type: pii.TypeEmail, Start: loc[0], End: loc[1]})
	}
	for _, loc := range phoneRe.FindAllStringIndex(text, -1) {
		frags = append(frags, detection.Fragment{Type: pii.TypePhone, Start: loc[0], End: loc[1]})
	}
	return detection.Merge(frags), nil
}