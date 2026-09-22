package detection

import (
	"testing"

	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

func TestMergeNonOverlapping(t *testing.T) {
	in := []Fragment{
		{Type: pii.TypeEmail, Start: 0, End: 5},
		{Type: pii.TypePhone, Start: 10, End: 15},
	}
	out := Merge(in)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
	if out[0].Start != 0 || out[1].Start != 10 {
		t.Fatalf("order wrong: %+v", out)
	}
}

func TestMergeOverlapKeepsLonger(t *testing.T) {
	in := []Fragment{
		{Type: pii.TypePhone, Start: 0, End: 10},
		{Type: pii.TypeCardNumber, Start: 2, End: 8},
	}
	out := Merge(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 merged fragment, got %d", len(out))
	}
	if out[0].Type != pii.TypePhone || out[0].Start != 0 || out[0].End != 10 {
		t.Fatalf("expected longer phone span, got %+v", out[0])
	}
}

func TestMergeUnsortedInput(t *testing.T) {
	in := []Fragment{
		{Type: pii.TypeEmail, Start: 20, End: 30},
		{Type: pii.TypePhone, Start: 0, End: 5},
	}
	out := Merge(in)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
	if out[0].Start != 0 || out[1].Start != 20 {
		t.Fatalf("not sorted: %+v", out)
	}
}

func TestMergeEmpty(t *testing.T) {
	if out := Merge(nil); out != nil {
		t.Fatalf("expected nil, got %+v", out)
	}
}