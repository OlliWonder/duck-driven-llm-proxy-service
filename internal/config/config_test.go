package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/duck-driven-llm-proxy-service/internal/masking"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

func TestLoadAndBuildPolicies(t *testing.T) {
	t.Setenv("PII_NER_ENDPOINT", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfgJSON := `{
	  "listen_addr": ":9090",
	  "aes_key": "",
	  "store_ttl": "1h",
	  "store_max_size": 100,
	  "ner_endpoint": "http://127.0.0.1:18090",
	  "allowlist": ["a", "b"],
	  "consumers": {
	    "a": {"enabled": true, "types": ["email"], "restore_allowed": true, "mode": "token"},
	    "b": {"enabled": true, "types": [], "restore_allowed": false, "mode": "mask"}
	  }
	}`
	if err := os.WriteFile(path, []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ListenAddr != ":9090" {
		t.Fatalf("listen addr: %q", cfg.ListenAddr)
	}
	if cfg.StoreTTL.D() != time.Hour {
		t.Fatalf("ttl: %v", cfg.StoreTTL.D())
	}
	if cfg.NEREndpoint != "http://127.0.0.1:18090" {
		t.Fatalf("NER endpoint: %q", cfg.NEREndpoint)
	}

	m, err := cfg.BuildPolicyManager()
	if err != nil {
		t.Fatalf("build policies: %v", err)
	}
	if !m.Allowed("a") || !m.Allowed("b") {
		t.Fatal("allowlisted consumers should be allowed")
	}
	if m.Allowed("c") {
		t.Fatal("non-allowlisted consumer should be rejected")
	}

	pa := m.For("a")
	if pa.Mode != masking.ModeToken || !pa.RestoreAllowed {
		t.Fatalf("policy a: %+v", pa)
	}
	if len(pa.Types) != 1 || pa.Types[0] != pii.TypeEmail {
		t.Fatalf("policy a types: %+v", pa.Types)
	}

	pb := m.For("b")
	if pb.Mode != masking.ModeMask || pb.RestoreAllowed {
		t.Fatalf("policy b: %+v", pb)
	}
}

func TestNEREndpointDefaultAndEnvironmentOverride(t *testing.T) {
	if got := Default().NEREndpoint; got != "http://127.0.0.1:8090" {
		t.Fatalf("default NER endpoint: %q", got)
	}
	t.Setenv("PII_NER_ENDPOINT", "http://ner-sidecar:8090")
	t.Setenv("PII_NER_WORKERS", "7")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.NEREndpoint != "http://ner-sidecar:8090" {
		t.Fatalf("environment NER endpoint: %q", cfg.NEREndpoint)
	}
	if cfg.NERWorkers != 7 {
		t.Fatalf("environment NER workers: %d", cfg.NERWorkers)
	}
}

func TestNEREndpointsEnvironmentOverride(t *testing.T) {
	t.Setenv("PII_NER_ENDPOINT", "http://ner-sidecar:8090")
	t.Setenv("PII_NER_ENDPOINTS", "http://ner-1:8090, http://ner-2:8090")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.NEREndpoints) != 2 {
		t.Fatalf("expected 2 NER endpoints, got %v", cfg.NEREndpoints)
	}
	if cfg.NEREndpoints[0] != "http://ner-1:8090" || cfg.NEREndpoints[1] != "http://ner-2:8090" {
		t.Fatalf("NER endpoints: %v", cfg.NEREndpoints)
	}
}

func TestInvalidMode(t *testing.T) {
	cfg := Default()
	cfg.Consumers = map[string]ConsumerConfig{
		"x": {Mode: "bogus"},
	}
	if _, err := cfg.BuildPolicyManager(); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestInvalidType(t *testing.T) {
	cfg := Default()
	cfg.Consumers = map[string]ConsumerConfig{
		"x": {Types: []string{"not_a_type"}},
	}
	if _, err := cfg.BuildPolicyManager(); err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestMaskModeRequiresNoRestore(t *testing.T) {
	cfg := Default()
	restore := true
	cfg.Consumers = map[string]ConsumerConfig{
		"x": {Mode: "mask", RestoreAllowed: &restore},
	}
	if _, err := cfg.BuildPolicyManager(); err == nil {
		t.Fatal("expected error: mask mode cannot allow restore")
	}
}
