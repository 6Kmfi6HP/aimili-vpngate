package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreEnsureAndUIConfig(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.EnsureWritable(); err != nil {
		t.Fatalf("EnsureWritable: %v", err)
	}
	cfg, err := store.LoadUIConfig("::", 8787, 7928)
	if err != nil {
		t.Fatalf("LoadUIConfig: %v", err)
	}
	if cfg.Port != 8787 || cfg.ProxyPort != 7928 || cfg.SecretPath == "" {
		t.Fatalf("unexpected cfg: %#v", cfg)
	}
	cfg.Password = "secret"
	if err := store.SaveUIConfig(cfg); err != nil {
		t.Fatalf("SaveUIConfig: %v", err)
	}
	loaded, err := store.LoadUIConfig("::", 8787, 7928)
	if err != nil {
		t.Fatalf("reload UI config: %v", err)
	}
	if loaded.Password != "secret" {
		t.Fatalf("password not preserved: %#v", loaded)
	}
}

func TestStateAndLogs(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.UpdateState(map[string]any{"status": "ok"}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	st, err := store.ReadState()
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if st["status"] != "ok" {
		t.Fatalf("state not saved: %#v", st)
	}
	if err := store.AppendLog("INFO", "Test", "hello"); err != nil {
		t.Fatalf("AppendLog: %v", err)
	}
	logs, err := store.ReadLogs()
	if err != nil {
		t.Fatalf("ReadLogs: %v", err)
	}
	if len(logs) != 1 || logs[0].Message != "hello" {
		t.Fatalf("unexpected logs: %#v", logs)
	}
}

func TestFixtureUIConfigCompatibility(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "vpngate_data", "ui_auth.json")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	store := NewStore(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.UIConfigFile(), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := store.LoadUIConfig("::", 8787, 7928)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SecretPath != "FixtureSecret12" || cfg.Username != "admin" {
		t.Fatalf("fixture not loaded: %#v", cfg)
	}
}
