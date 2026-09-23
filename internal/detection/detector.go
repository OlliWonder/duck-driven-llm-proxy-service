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
	"sort"

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

// NewRuleBasedDetector создаёт полный правиловый детектор. Единый реестр
// позволяет добавить простой тип одной строкой и даёт сервису стабильный
// конструктор для интеграции.
func NewRuleBasedDetector() Detector {
	return NewComposite(
		NewCardHolderDetector(), // more specific than a generic name field
		NewFullNameDetector(),
		NewBirthDateDetector(),
		NewBirthPlaceDetector(),
		NewPassportDetector(),
		NewCitizenshipDetector(),
		NewPassportIssuerDetector(),
		NewPassportDeptCodeDetector(),
		NewPassportIssueDateDetector(),
		NewDrivingLicenseDetector(),
		NewAddressDetector(),
		NewEmailDetector(),
		NewPhoneDetector(),
		NewINNDetector(),
		NewCVVDetector(),
		NewPINDetector(),
		NewCardNumberDetector(),
	)
}

// Detect запускает все детекторы и объединяет результаты. Более длинный span
// побеждает при пересечении; для одинаковых span сохраняется порядок
// детекторов в Composite.
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
	// Сначала выбираем более длинные span. SliceStable сохраняет приоритет
	// детекторов при равных span (например, card_holder перед full_name).
	sort.SliceStable(fragments, func(i, j int) bool {
		leftLen := fragments[i].End - fragments[i].Start
		rightLen := fragments[j].End - fragments[j].Start
		if leftLen != rightLen {
			return leftLen > rightLen
		}
		return fragments[i].Start < fragments[j].Start
	})
	selected := make([]Fragment, 0, len(fragments))
	for _, candidate := range fragments {
		overlaps := false
		for _, existing := range selected {
			if candidate.Start < existing.End && existing.Start < candidate.End {
				overlaps = true
				break
			}
		}
		if !overlaps {
			selected = append(selected, candidate)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Start < selected[j].Start })
	return selected
}
