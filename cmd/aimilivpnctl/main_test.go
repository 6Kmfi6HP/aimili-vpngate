package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveUIConfigPreservesUnknownAndRoutingFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPNGATE_DATA_DIR", dir)
	initial := `{
  "host": "::",
  "port": 8787,
  "proxy_port": 7928,
  "secret_path": "secret",
  "username": "admin",
  "password": "old",
  "routing_mode": "fixed_region",
  "force_country": "JP",
  "fixed_node_id": "node-1",
  "future_field": "keep"
}
`
	if err := os.WriteFile(filepath.Join(dir, "ui_auth.json"), []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadUIConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Password = "new"
	if err := saveUIConfig(cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "ui_auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"routing_mode": "fixed_region"`, `"force_country": "JP"`, `"fixed_node_id": "node-1"`, `"future_field": "keep"`, `"password": "new"`} {
		if !containsString(string(raw), want) {
			t.Fatalf("saved config missing %s:\n%s", want, raw)
		}
	}
}

func TestPreferredLogFileUsesStructuredGoLogFirst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPNGATE_DATA_DIR", dir)
	t.Setenv("AIMILIVPN_LOG_DAY", "2026-01-02")
	if err := os.WriteFile(filepath.Join(dir, "vpngate.log"), []byte("legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	goLog := filepath.Join(dir, "logs", "2026-01-02.json")
	if err := os.WriteFile(goLog, []byte(`{"timestamp":"2026-01-02 03:04:05","message":"go"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := preferredLogFile(); got != goLog {
		t.Fatalf("preferredLogFile = %q, want %q", got, goLog)
	}
}

func containsString(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
