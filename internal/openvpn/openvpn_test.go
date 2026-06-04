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

func TestBuildCommandIncludesParityFlagsAndTCPProxy(t *testing.T) {
	t.Setenv("OPENVPN_UPSTREAM_SOCKS", "127.0.0.1:1080")
	exe, args := BuildCommand(config.Config{OpenVPNCmd: "openvpn"}, "/tmp/auth.txt", CommandOptions{
		ConfigPath:  "/tmp/node.ovpn",
		Device:      "tun7",
		RouteNoPull: true,
		ConfigText:  "client\nproto tcp\nremote 198.51.100.10 443\n",
	})
	if exe != "openvpn" {
		t.Fatalf("exe = %q", exe)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--config /tmp/node.ovpn",
		"--dev tun7",
		"--dev-type tun",
		"--pull-filter ignore route-ipv6",
		"--pull-filter ignore ifconfig-ipv6",
		"--connect-retry-max 1",
		"--connect-timeout 15",
		"--auth-user-pass /tmp/auth.txt",
		"--auth-nocache",
		"--data-ciphers",
		"--route-nopull",
		"--socks-proxy 127.0.0.1 1080",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("command missing %q:\n%s", want, joined)
		}
	}
}

func TestProbeReportsReadinessDiagnostics(t *testing.T) {
	dir := t.TempDir()
	fakeOpenVPN := filepath.Join(dir, "fake-openvpn")
	script := `#!/bin/sh
printf '%s\n' 'AUTH_FAILED' >&2
`
	if err := os.WriteFile(fakeOpenVPN, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(config.Config{
		OpenVPNCmd:                fakeOpenVPN,
		OpenVPNAuthUser:           "vpn",
		OpenVPNAuthPass:           "vpn",
		OpenVPNTestTimeoutSeconds: 1,
	}, filepath.Join(dir, "auth.txt"), nil)
	res := manager.Probe(context.Background(), vpngate.Node{ID: "n"}, filepath.Join(dir, "client.ovpn"), "tun2", time.Second)
	if res.OK || !strings.Contains(res.Message, "ERR_OVPN_AUTH_FAILED") {
		t.Fatalf("unexpected probe result: %#v", res)
	}
}

func TestProbeTimeoutReportsDiagnostic(t *testing.T) {
	dir := t.TempDir()
	fakeOpenVPN := filepath.Join(dir, "fake-openvpn")
	script := `#!/bin/sh
sleep 5
`
	if err := os.WriteFile(fakeOpenVPN, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(config.Config{
		OpenVPNCmd:                fakeOpenVPN,
		OpenVPNAuthUser:           "vpn",
		OpenVPNAuthPass:           "vpn",
		OpenVPNTestTimeoutSeconds: 1,
	}, filepath.Join(dir, "auth.txt"), nil)
	res := manager.Probe(context.Background(), vpngate.Node{ID: "n"}, filepath.Join(dir, "client.ovpn"), "tun2", 50*time.Millisecond)
	if res.OK || !strings.Contains(res.Message, "ERR_OVPN_TIMEOUT") {
		t.Fatalf("unexpected timeout result: %#v", res)
	}
}

func TestDiagnoseOpenVPNTunVariants(t *testing.T) {
	handled, err := diagnoseOpenVPNLine("ERROR: Cannot ioctl TUNSETIFF tun: Operation not permitted")
	if !handled || err == nil || !strings.Contains(err.Error(), "ERR_OVPN_TUN_NOT_AVAILABLE") {
		t.Fatalf("unexpected tun diagnosis handled=%v err=%v", handled, err)
	}
}

func TestDiagnoseOpenVPNNetIfaceUpAsReady(t *testing.T) {
	handled, err := diagnoseOpenVPNLine("2026-06-04 15:25:35 net_iface_up: set tun6 up")
	if !handled || err != nil {
		t.Fatalf("unexpected readiness diagnosis handled=%v err=%v", handled, err)
	}
}

func TestDiagnoseOpenVPNTunOpenedIsNotFailure(t *testing.T) {
	handled, err := diagnoseOpenVPNLine("2026-06-04 15:25:35 TUN/TAP device tun6 opened")
	if handled || err != nil {
		t.Fatalf("tun opened should not be terminal diagnosis handled=%v err=%v", handled, err)
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
