// Пакет masking реализует токенизацию (маскирование) персональных данных и
// их точное восстановление (демаскирование).
//
// Контракт:
//   - Фрагменты ссылаются на байтовые смещения (UTF-8) в исходной строке,
//     start включительно, end исключительно.
//   - Маскирование заменяет каждый фрагмент обратимым токеном. Исходные
//     фрагменты сохраняются дословно (никогда не нормализуются).
//   - Восстановление заменяет токены обратно на точные исходные байты,
//     поэтому результат совпадает с оригиналом посимвольно.
//   - Токены содержат nonce операции, поэтому не могут совпасть с
//     естественным текстом и уникальны в рамках одной операции маскирования.
package masking

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// TokenMapping связывает токен в замаскированном тексте с его исходным фрагментом.
type TokenMapping struct {
	Token    string
	Original string
	Type     pii.Type
}

// Masker создаёт замаскированный текст из обнаруженных фрагментов.
type Masker struct {
	// Mode управляет тем, как фрагменты отображаются в замаскированном выводе.
	Mode Mode
}

// Mode выбирает стиль маскирования.
type Mode string

const (
	// ModeToken заменяет каждый фрагмент обратимым токеном.
	ModeToken Mode = "token"
	// ModeMask заменяет каждый фрагмент маской фиксированной длины (например, "***").
	// Этот режим НЕ обратим; восстановление для него отключено.
	ModeMask Mode = "mask"
)

// Result — результат операции маскирования.
type Result struct {
	Masked   string
	Mappings []TokenMapping
}

// NewMasker возвращает Masker с заданным режимом.
func NewMasker(mode Mode) *Masker {
	return &Masker{Mode: mode}
}

// Mask заменяет переданные фрагменты в text замаскированными формами.
// Возвращает замаскированный текст и соответствия токенов, необходимые для
// восстановления.
func (m *Masker) Mask(text string, fragments []detection.Fragment) (Result, error) {
	if len(fragments) == 0 {
		return Result{Masked: text}, nil
	}
	nonce, err := randomNonce()
	if err != nil {
		return Result{}, err
	}

	// Сортируем фрагменты по start, чтобы идти по строке слева направо.
	sorted := detection.Merge(fragments)

	var b strings.Builder
	prev := 0
	mappings := make([]TokenMapping, 0, len(sorted))
	for i, f := range sorted {
		if f.Start < prev || f.End > len(text) || f.Start > f.End {
			return Result{}, errors.New("masking: fragment out of bounds")
		}
		// Копируем промежуток перед этим фрагментом.
		b.WriteString(text[prev:f.Start])
		original := text[f.Start:f.End]

		switch m.Mode {
		case ModeToken:
			token := tokenFor(f.Type, i, nonce)
			b.WriteString(token)
			mappings = append(mappings, TokenMapping{Token: token, Original: original, Type: f.Type})
		case ModeMask:
			b.WriteString(maskFor(f.Type))
		default:
			return Result{}, errors.New("masking: unknown mode")
		}
		prev = f.End
	}
	b.WriteString(text[prev:])

	return Result{Masked: b.String(), Mappings: mappings}, nil
}

// Restore заменяет токены в maskedText их исходными фрагментами.
// Возвращает ошибку, если токен не найден в соответствиях.
func Restore(maskedText string, mappings []TokenMapping) (string, error) {
	if len(mappings) == 0 {
		return maskedText, nil
	}
	byToken := make(map[string]string, len(mappings))
	for _, mp := range mappings {
		byToken[mp.Token] = mp.Original
	}
	var b strings.Builder
	rest := maskedText
	for {
		idx := strings.Index(rest, tokenPrefix)
		if idx < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:idx])
		rest = rest[idx:]
		end := strings.Index(rest, tokenSuffix)
		if end < 0 {
			return "", errors.New("masking: unterminated token")
		}
		end += len(tokenSuffix)
		tok := rest[:end]
		orig, ok := byToken[tok]
		if !ok {
			return "", errors.New("masking: unknown token in masked text")
		}
		b.WriteString(orig)
		rest = rest[end:]
	}
	return b.String(), nil
}

const (
	tokenPrefix = "⟦ПД:"
	tokenSuffix = "⟧"
)

func tokenFor(t pii.Type, index int, nonce string) string {
	return tokenPrefix + string(t) + ":" + itoa(index) + ":" + nonce + tokenSuffix
}

func maskFor(t pii.Type) string {
	// Маска фиксированной длины; необратима.
	return "***"
}

func randomNonce() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}