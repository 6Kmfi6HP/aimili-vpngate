package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("VPNGATE_DATA_DIR", "")
	cfg := Load("test")
	if cfg.LocalProxyHost != "127.0.0.1" {
		t.Fatalf("LocalProxyHost = %q", cfg.LocalProxyHost)
	}
	if cfg.LocalProxyPort != 7928 {
		t.Fatalf("LocalProxyPort = %d", cfg.LocalProxyPort)
	}
	if cfg.UIPort != 8787 {
		t.Fatalf("UIPort = %d", cfg.UIPort)
	}
	if cfg.OpenVPNCmd != "openvpn" {
		t.Fatalf("OpenVPNCmd = %q", cfg.OpenVPNCmd)
	}
	if cfg.LocalProxyOutboundDevice != "tun0" {
		t.Fatalf("LocalProxyOutboundDevice = %q", cfg.LocalProxyOutboundDevice)
	}
	if !cfg.AutoConnect {
		t.Fatal("AutoConnect default is false")
	}
	if !cfg.IPEnrichment {
		t.Fatal("IPEnrichment default is false")
	}
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPNGATE_DATA_DIR", dir)
	t.Setenv("LOCAL_PROXY_HOST", "::")
	t.Setenv("LOCAL_PROXY_PORT", "18080")
	t.Setenv("UI_HOST", "127.0.0.1")
	t.Setenv("UI_PORT", "19090")
	t.Setenv("MAX_SCAN_ROWS", "7")
	t.Setenv("LOCAL_PROXY_OUTBOUND_DEVICE", "none")
	t.Setenv("AIMILIVPN_AUTOCONNECT", "false")
	t.Setenv("AIMILIVPN_IP_ENRICHMENT", "false")
	cfg := Load("test")
	if cfg.DataDir != dir {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, dir)
	}
	if cfg.LocalProxyHost != "::" || cfg.LocalProxyPort != 18080 {
		t.Fatalf("proxy override not applied: %#v", cfg)
	}
	if cfg.UIHost != "127.0.0.1" || cfg.UIPort != 19090 {
		t.Fatalf("ui override not applied: %#v", cfg)
	}
	if cfg.MaxScanRows != 7 {
		t.Fatalf("MaxScanRows = %d", cfg.MaxScanRows)
	}
	if cfg.LocalProxyOutboundDevice != "" {
		t.Fatalf("LocalProxyOutboundDevice = %q", cfg.LocalProxyOutboundDevice)
	}
	if cfg.AutoConnect {
		t.Fatal("AutoConnect override not applied")
	}
	if cfg.IPEnrichment {
		t.Fatal("IPEnrichment override not applied")
	}
}

func TestContainerDataDirDefault(t *testing.T) {
	t.Setenv("VPNGATE_DATA_DIR", "")
	t.Setenv("AIMILIVPN_CONTAINER", "true")
	cfg := Load("test")
	if cfg.DataDir != "/data" {
		t.Fatalf("container DataDir = %q", cfg.DataDir)
	}
}

func TestExplicitRelativeDataDirBecomesAbsolute(t *testing.T) {
	rel := filepath.Join("testdata", "relative-data")
	t.Setenv("VPNGATE_DATA_DIR", rel)
	cfg := Load("test")
	if !filepath.IsAbs(cfg.DataDir) {
		t.Fatalf("DataDir is not absolute: %q", cfg.DataDir)
	}
	_ = os.RemoveAll(cfg.DataDir)
}
