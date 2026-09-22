package masking

import (
	"testing"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

func TestMaskRestoreRoundtrip(t *testing.T) {
	text := "Клиент Иванов Иван, email ivan@example.com, тел +7 900 123 45 67"
	frags := []detection.Fragment{
		{Type: pii.TypeEmail, Start: 27, End: 42},
		{Type: pii.TypePhone, Start: 49, End: 65},
	}
	m := NewMasker(ModeToken)
	res, err := m.Mask(text, frags)
	if err != nil {
		t.Fatalf("mask: %v", err)
	}
	if res.Masked == text {
		t.Fatal("masked text equals original")
	}
	got, err := Restore(res.Masked, res.Mappings)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got != text {
		t.Fatalf("roundtrip mismatch:\n got %q\nwant %q", got, text)
	}
}

func TestMaskRestoreUnicode(t *testing.T) {
	// Кириллические символы многобайтовы в UTF-8; байтовые смещения должны
	// соблюдаться, чтобы восстановление было точным.
	text := "Привет, Иван Иванович, ваш email: test@mail.ru"
	// email начинается с байтового смещения 't' в "test@mail.ru"
	emailStart := len("Привет, Иван Иванович, ваш email: ")
	frags := []detection.Fragment{
		{Type: pii.TypeEmail, Start: emailStart, End: len(text)},
	}
	m := NewMasker(ModeToken)
	res, err := m.Mask(text, frags)
	if err != nil {
		t.Fatalf("mask: %v", err)
	}
	got, err := Restore(res.Masked, res.Mappings)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got != text {
		t.Fatalf("unicode roundtrip mismatch:\n got %q\nwant %q", got, text)
	}
}

func TestMaskNoFragments(t *testing.T) {
	m := NewMasker(ModeToken)
	res, err := m.Mask("просто текст без пд", nil)
	if err != nil {
		t.Fatalf("mask: %v", err)
	}
	if res.Masked != "просто текст без пд" {
		t.Fatalf("expected unchanged text, got %q", res.Masked)
	}
	if len(res.Mappings) != 0 {
		t.Fatalf("expected no mappings, got %d", len(res.Mappings))
	}
}

func TestRestoreUnknownToken(t *testing.T) {
	_, err := Restore("⟦ПД:email:0:deadbeef⟧", []TokenMapping{
		{Token: "⟦ПД:email:0:aaaaaa⟧", Original: "x"},
	})
	if err == nil {
		t.Fatal("expected error for unknown token")
	}
}

func TestMaskModeMaskNotReversible(t *testing.T) {
	m := NewMasker(ModeMask)
	res, err := m.Mask("email test@mail.ru", []detection.Fragment{
		{Type: pii.TypeEmail, Start: 6, End: 18},
	})
	if err != nil {
		t.Fatalf("mask: %v", err)
	}
	if res.Masked != "email ***" {
		t.Fatalf("expected fixed mask, got %q", res.Masked)
	}
	if len(res.Mappings) != 0 {
		t.Fatalf("mask mode must not produce mappings, got %d", len(res.Mappings))
	}
}

func TestMaskOutOfBounds(t *testing.T) {
	m := NewMasker(ModeToken)
	_, err := m.Mask("abc", []detection.Fragment{
		{Type: pii.TypeEmail, Start: 0, End: 10},
	})
	if err == nil {
		t.Fatal("expected error for out-of-bounds fragment")
	}
}