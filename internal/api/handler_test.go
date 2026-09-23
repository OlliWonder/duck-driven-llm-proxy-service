package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
	"github.com/duck-driven-llm-proxy-service/internal/metrics"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
	"github.com/duck-driven-llm-proxy-service/internal/policy"
	"github.com/duck-driven-llm-proxy-service/internal/store"
)

func newTestHandler() *Handler {
	key := bytes.Repeat([]byte{0x42}, 32)
	st, _ := store.New(key, time.Hour, 1000)
	pol := policy.NewManager()
	pol.SetPolicy("default", policy.Default())
	det := &fakeDetector{frags: []detection.Fragment{
		{Type: pii.TypeEmail, Start: 6, End: 18},
	}}
	m := metrics.New()
	svc := NewService(det, st, pol, m)
	return NewHandler(svc, m, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
}

func TestHandlerMaskDemask(t *testing.T) {
	h := newTestHandler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := `{"payload":"email test@mail.ru","payload_id":"h-1"}`
	resp, err := http.Post(srv.URL+"/process", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var maskResp ProcessResponse
	if err := json.NewDecoder(resp.Body).Decode(&maskResp); err != nil {
		t.Fatal(err)
	}
	if maskResp.Result == "email test@mail.ru" {
		t.Fatal("mask equals original")
	}

	// Демаскирование.
	demaskBody, _ := json.Marshal(ProcessRequest{Payload: maskResp.Result, PayloadID: "h-1"})
	resp2, err := http.Post(srv.URL+"/process", "application/json", bytes.NewBuffer(demaskBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var origResp ProcessResponse
	if err := json.NewDecoder(resp2.Body).Decode(&origResp); err != nil {
		t.Fatal(err)
	}
	if origResp.Result != "email test@mail.ru" {
		t.Fatalf("demask mismatch: %q", origResp.Result)
	}
}

func TestHandlerInvalidJSON(t *testing.T) {
	h := newTestHandler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/process", "application/json", bytes.NewBufferString("{bad"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlerRejectsMultipleJSONValues(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(
		`{"payload":"x","payload_id":"one"}{"payload":"y","payload_id":"two"}`,
	))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandlerRejectsOversizedBody(t *testing.T) {
	h := newTestHandler()
	body := `{"payload":"` + strings.Repeat("a", int(maxProcessBodyBytes)) + `","payload_id":"large"}`
	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", w.Code)
	}
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	h := newTestHandler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/process")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestHandlerRateLimit(t *testing.T) {
	h := newTestHandler()
	h.rateLimiter = func() bool { return false }
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/process", "application/json", bytes.NewBufferString(`{"payload":"x","payload_id":"r-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}
