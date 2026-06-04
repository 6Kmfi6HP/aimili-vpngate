package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/6Kmfi6HP/aimili-vpngate/internal/config"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/state"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/vpngate"
)

func TestNodesPreservesConnectingState(t *testing.T) {
	app, err := New(config.Config{
		DataDir:        t.TempDir(),
		UIHost:         "127.0.0.1",
		UIPort:         8787,
		LocalProxyPort: 7928,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.UpdateState(map[string]any{"is_connecting": true}); err != nil {
		t.Fatal(err)
	}
	_, runtimeState, err := app.Nodes()
	if err != nil {
		t.Fatal(err)
	}
	if runtimeState["is_connecting"] != true {
		t.Fatalf("is_connecting was not preserved: %#v", runtimeState)
	}
}

func TestCheckNodesSerializesProbeBatches(t *testing.T) {
	dir := t.TempDir()
	fakeOpenVPN := filepath.Join(dir, "fake-openvpn")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '%s\n' 'OpenVPN 2.6.0'
  exit 0
fi
lock="${AIMILIVPN_TEST_LOCK}"
log="${AIMILIVPN_TEST_LOG}"
if mkdir "$lock" 2>/dev/null; then
  trap 'rmdir "$lock" 2>/dev/null || true' EXIT INT TERM
else
  printf '%s\n' concurrent >> "$log"
fi
sleep 0.2
rmdir "$lock" 2>/dev/null || true
printf '%s\n' 'Initialization Sequence Completed'
sleep 0.05
`
	if err := os.WriteFile(fakeOpenVPN, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(dir, "probe.log")
	t.Setenv("AIMILIVPN_TEST_LOCK", filepath.Join(dir, "probe.lock"))
	t.Setenv("AIMILIVPN_TEST_LOG", logFile)

	app, err := New(config.Config{
		DataDir:                   filepath.Join(dir, "data"),
		UIHost:                    "127.0.0.1",
		UIPort:                    8787,
		LocalProxyPort:            7928,
		OpenVPNCmd:                fakeOpenVPN,
		OpenVPNAuthUser:           "vpn",
		OpenVPNAuthPass:           "vpn",
		OpenVPNTestTimeoutSeconds: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	node := func(id string) vpngate.Node {
		return vpngate.Node{
			ID:          id,
			RemoteHost:  "203.0.113.1",
			RemotePort:  1194,
			RemoteProto: "udp",
			ConfigText:  "client\nproto udp\nremote 203.0.113.1 1194\n",
		}
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, nodes := range [][]vpngate.Node{{node("one")}, {node("two")}} {
		wg.Add(1)
		go func(nodes []vpngate.Node) {
			defer wg.Done()
			<-start
			_ = app.checkNodes(context.Background(), nodes, 1)
		}(nodes)
	}
	close(start)
	wg.Wait()

	raw, err := os.ReadFile(logFile)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(raw) > 0 {
		t.Fatalf("probe batches overlapped:\n%s", raw)
	}
}

func TestSetupPolicyRoutingRetriesTransientFailures(t *testing.T) {
	oldRunner := commandRunner
	oldDelay := policyRetryDelay
	policyRetryDelay = time.Millisecond
	routeAddAttempts := 0
	commandRunner = func(_ context.Context, name string, args ...string) error {
		if name == "ip" && strings.Join(args, " ") == "route add default dev tun0 table 100" {
			routeAddAttempts++
			if routeAddAttempts < 3 {
				return errors.New("transient route failure")
			}
		}
		return nil
	}
	t.Cleanup(func() {
		commandRunner = oldRunner
		policyRetryDelay = oldDelay
	})

	app := &App{store: state.NewStore(t.TempDir())}
	if err := app.store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := app.setupPolicyRouting("tun0"); err != nil {
		t.Fatal(err)
	}
	if routeAddAttempts != 3 {
		t.Fatalf("route add attempts = %d", routeAddAttempts)
	}
}

func TestHeartbeatStatusUsesStartupGrace(t *testing.T) {
	app, err := New(config.Config{
		DataDir:        t.TempDir(),
		UIHost:         "127.0.0.1",
		UIPort:         8787,
		LocalProxyPort: 7928,
	})
	if err != nil {
		t.Fatal(err)
	}
	if app.startTime.IsZero() {
		t.Fatal("startTime was not recorded")
	}
	if got := app.heartbeatStatus(time.Now(), time.Second, 15*time.Second); got != "starting" {
		t.Fatalf("startup heartbeat = %q", got)
	}
	app.startTime = time.Now().Add(-40 * time.Second)
	if got := app.heartbeatStatus(time.Now(), time.Second, 15*time.Second); got != "running" {
		t.Fatalf("fresh heartbeat = %q", got)
	}
	if got := app.heartbeatStatus(time.Now().Add(-2*time.Second), time.Second, 15*time.Second); got != "stopped" {
		t.Fatalf("stale heartbeat = %q", got)
	}
}
