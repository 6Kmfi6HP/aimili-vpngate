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
	fakeOpenVPN := fakeOpenVPNVersion(t, "OpenVPN 2.6.9 x86_64-pc-linux-gnu\n")
	exe, args := BuildCommand(config.Config{OpenVPNCmd: fakeOpenVPN}, "/tmp/auth.txt", CommandOptions{
		ConfigPath:  "/tmp/node.ovpn",
		Device:      "tun7",
		RouteNoPull: true,
		ConfigText:  "client\nproto tcp\nremote 198.51.100.10 443\n",
	})
	if exe != fakeOpenVPN {
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

func TestBuildCommandUsesNCPCiphersForOpenVPN24(t *testing.T) {
	fakeOpenVPN := fakeOpenVPNVersion(t, "OpenVPN 2.4.12 x86_64-pc-linux-gnu\n")
	_, args := BuildCommand(config.Config{OpenVPNCmd: fakeOpenVPN}, "/tmp/auth.txt", CommandOptions{ConfigPath: "/tmp/node.ovpn"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--ncp-ciphers") {
		t.Fatalf("ncp-ciphers missing:\n%s", joined)
	}
	if strings.Contains(joined, "--data-ciphers") {
		t.Fatalf("data-ciphers should not be used for 2.4:\n%s", joined)
	}
}

func TestDetectOpenVPNVersionDefaultsTo24OnFailure(t *testing.T) {
	resetVersionCache(t)
	if got := DetectOpenVPNVersion(config.Config{OpenVPNCmd: filepath.Join(t.TempDir(), "missing-openvpn")}); got != "2.4" {
		t.Fatalf("version = %q", got)
	}
}

func TestProbeReportsReadinessDiagnostics(t *testing.T) {
	dir := t.TempDir()
	fakeOpenVPN := filepath.Join(dir, "fake-openvpn")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '%s\n' 'OpenVPN 2.6.0'
  exit 0
fi
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
if [ "$1" = "--version" ]; then
  printf '%s\n' 'OpenVPN 2.6.0'
  exit 0
fi
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

func TestDiagnoseOpenVPNNodeUnreachableAndUnknown(t *testing.T) {
	handled, err := diagnoseOpenVPNLine("TCP: connect to [AF_INET]203.0.113.10:443 failed: Connection refused")
	if !handled || err == nil || !strings.Contains(err.Error(), "[2004] ERR_OVPN_NODE_UNREACHABLE") {
		t.Fatalf("unexpected unreachable diagnosis handled=%v err=%v", handled, err)
	}
	handled, err = diagnoseOpenVPNLine("Exiting due to fatal error")
	if !handled || err == nil || !strings.Contains(err.Error(), "[2010] ERR_OVPN_UNKNOWN") {
		t.Fatalf("unexpected unknown diagnosis handled=%v err=%v", handled, err)
	}
}

func TestStartKeepsProcessAfterCallerContextIsCanceled(t *testing.T) {
	dir := t.TempDir()
	fakeOpenVPN := filepath.Join(dir, "fake-openvpn")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '%s\n' 'OpenVPN 2.6.0'
  exit 0
fi
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

func TestKillOrphanProcessesRunsExpectedPkillPatterns(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "pkill.log")
	pkill := filepath.Join(dir, "pkill")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "` + logFile + `"
exit 0
`
	if err := os.WriteFile(pkill, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := KillOrphanProcesses(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"-f openvpn.*tun0", "-f openvpn.*vpngate_data"} {
		if !strings.Contains(text, want) {
			t.Fatalf("pkill log missing %q:\n%s", want, text)
		}
	}
}

func fakeOpenVPNVersion(t *testing.T, versionOutput string) string {
	t.Helper()
	resetVersionCache(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-openvpn")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '%s' "` + versionOutput + `"
  exit 0
fi
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func resetVersionCache(t *testing.T) {
	t.Helper()
	versionMu.Lock()
	old := versionCache
	versionCache = map[string]string{}
	versionMu.Unlock()
	t.Cleanup(func() {
		versionMu.Lock()
		versionCache = old
		versionMu.Unlock()
	})
}
