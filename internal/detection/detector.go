// Пакет detection определяет контракт, которому должен удовлетворять каждый
// детектор ПД. Участник 2 владеет конкретными реализациями; участник 1
// потребляет этот интерфейс из слоя маскирования.
//
// Контракт (фиксированный):
//   - Фрагменты несут байтовые смещения в ИСХОДНОЙ строке UTF-8.
//   - Start включительно, End исключительно.
//   - Детекторы возвращают только позиции; они никогда не меняют текст.
//   - Фрагменты не должны пересекаться (один участок не может быть
//     одновременно телефоном и номером карты).
package detection

import (
	"context"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// Fragment — обнаруженный участок персональных данных в исходном тексте.
type Fragment struct {
	Type pii.Type
	// Start — включительное байтовое смещение (UTF-8) в исходной строке.
	Start int
	// End — исключительное байтовое смещение (UTF-8) в исходной строке.
	End int
}

// Detector идентифицирует участки персональных данных в тексте.
type Detector interface {
	// Detect возвращает непересекающиеся фрагменты для данного текста.
	// Возвращённые фрагменты ссылаются на байтовые смещения в text.
	// Детектор должен возвращать ошибку, когда требуемая модель недоступна,
	// а не молча сообщать «данных нет».
	Detect(ctx context.Context, text string) ([]Fragment, error)
}

// Composite запускает несколько детекторов и объединяет их фрагменты в
// единый непересекающийся набор. Используется для комбинации правиловых
// детекторов с локальной NER-моделью.
type Composite struct {
	detectors []Detector
}

// NewComposite создаёт композит из переданных детекторов.
func NewComposite(detectors ...Detector) *Composite {
	return &Composite{detectors: detectors}
}

// Detect запускает все детекторы и объединяет результаты, разрешая
// пересечения в пользу первого детектора, сообщившего об участке.
func (c *Composite) Detect(ctx context.Context, text string) ([]Fragment, error) {
	var all []Fragment
	for _, d := range c.detectors {
		frags, err := d.Detect(ctx, text)
		if err != nil {
			return nil, err
		}
		all = append(all, frags...)
	}
	return Merge(all), nil
}

// Merge объединяет фрагменты в единый непересекающийся отсортированный
// список. При пересечении двух фрагментов побеждает более длинный; при
// равенстве — тот, что начинается раньше. Это гарантирует, что участок
// никогда не заменяется дважды (например, и как телефон, и как номер карты).
func Merge(fragments []Fragment) []Fragment {
	if len(fragments) == 0 {
		return nil
	}
	// Сортировка по start по возрастанию, затем по end по убыванию (длиннее первым).
	sortFragments(fragments)

	out := make([]Fragment, 0, len(fragments))
	for _, f := range fragments {
		if len(out) == 0 {
			out = append(out, f)
			continue
		}
		last := &out[len(out)-1]
		if f.Start < last.End {
			// Пересечение: оставляем уже находящийся в out более длинный участок.
			continue
		}
		out = append(out, f)
	}
	return out
}

func sortFragments(fs []Fragment) {
	for i := 1; i < len(fs); i++ {
		for j := i; j > 0; j-- {
			a, b := fs[j-1], fs[j]
			if a.Start < b.Start || (a.Start == b.Start && a.End > b.End) {
				break
			}
			fs[j-1], fs[j] = b, a
		}
	}
}