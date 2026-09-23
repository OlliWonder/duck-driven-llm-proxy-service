package tempdetect

import (
	"context"
	"testing"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

func TestDetectEmail(t *testing.T) {
	d := New()
	frags, err := d.Detect(context.Background(), "свяжитесь: test@mail.ru")
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 1 {
		t.Fatalf("expected 1 fragment, got %d", len(frags))
	}
	if frags[0].Type != pii.TypeEmail {
		t.Fatalf("expected email type, got %s", frags[0].Type)
	}
	got := "свяжитесь: test@mail.ru"[frags[0].Start:frags[0].End]
	if got != "test@mail.ru" {
		t.Fatalf("span mismatch: %q", got)
	}
}

func TestDetectPhone(t *testing.T) {
	d := New()
	frags, err := d.Detect(context.Background(), "тел +7 900 123 45 67")
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 1 {
		t.Fatalf("expected 1 fragment, got %d", len(frags))
	}
	if frags[0].Type != pii.TypePhone {
		t.Fatalf("expected phone type, got %s", frags[0].Type)
	}
}

func TestDetectNone(t *testing.T) {
	d := New()
	frags, err := d.Detect(context.Background(), "просто текст")
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 0 {
		t.Fatalf("expected 0 fragments, got %d", len(frags))
	}
}