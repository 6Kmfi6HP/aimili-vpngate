package openvpn

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/6Kmfi6HP/aimili-vpngate/internal/config"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/vpngate"
)

func TestNormalizeConfigAddsOpenVPN26Ciphers(t *testing.T) {
	got := NormalizeConfig("client\ndev tun\ncipher AES-128-CBC\n")
	if !strings.Contains(got, "data-ciphers ") {
		t.Fatalf("data-ciphers missing:\n%s", got)
	}
	if !strings.Contains(got, "dev tun0") {
		t.Fatalf("dev tun0 missing:\n%s", got)
	}
	if !strings.Contains(got, "data-ciphers-fallback AES-128-CBC") {
		t.Fatalf("fallback missing:\n%s", got)
	}
}

func TestNormalizeConfigDoesNotDuplicateCiphers(t *testing.T) {
	input := "client\ndata-ciphers AES-128-CBC\ndata-ciphers-fallback AES-128-CBC\n"
	got := NormalizeConfig(input)
	if strings.Count(got, "data-ciphers ") != 1 {
		t.Fatalf("data-ciphers duplicated:\n%s", got)
	}
	if strings.Count(got, "data-ciphers-fallback ") != 1 {
		t.Fatalf("fallback duplicated:\n%s", got)
	}
}

func TestWriteConfigNormalizesConfig(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteConfig(dir, vpngate.Node{ID: "node", ConfigText: "client\ncipher AES-128-CBC\n"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "data-ciphers-fallback AES-128-CBC") {
		t.Fatalf("config not normalized:\n%s", raw)
	}
}

func TestStartKeepsProcessAfterCallerContextIsCanceled(t *testing.T) {
	dir := t.TempDir()
	fakeOpenVPN := filepath.Join(dir, "fake-openvpn")
	script := `#!/bin/sh
printf '%s\n' 'Initialization Sequence Completed'
trap 'exit 0' INT TERM
while true; do sleep 1; done
`
	if err := os.WriteFile(fakeOpenVPN, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(config.Config{
		OpenVPNCmd:                fakeOpenVPN,
		OpenVPNAuthUser:           "vpn",
		OpenVPNAuthPass:           "vpn",
		OpenVPNTestTimeoutSeconds: 2,
	}, filepath.Join(dir, "auth.txt"), nil)
	t.Cleanup(manager.Stop)

	ctx, cancel := context.WithCancel(context.Background())
	if err := manager.Start(ctx, vpngate.Node{ID: "node"}, filepath.Join(dir, "client.ovpn")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancel()
	time.Sleep(200 * time.Millisecond)
	if !manager.Running() {
		t.Fatal("OpenVPN process stopped when caller context was canceled")
	}
}
