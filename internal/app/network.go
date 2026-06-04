package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func (a *App) setupPolicyRouting(iface string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	_ = runCommand(context.Background(), "ip", "rule", "del", "table", "100")
	_ = runCommand(context.Background(), "ip", "route", "flush", "table", "100")
	if err := runCommand(context.Background(), "ip", "route", "add", "default", "dev", iface, "table", "100"); err != nil {
		return fmt.Errorf("[ERR_ROUTE_TABLE_ADD_FAILED] %w", err)
	}
	if err := runCommand(context.Background(), "ip", "rule", "add", "oif", iface, "table", "100"); err != nil {
		return fmt.Errorf("[ERR_ROUTE_RULE_ADD_FAILED] %w", err)
	}
	for _, target := range []string{"all", "default", iface} {
		_ = runCommand(context.Background(), "sysctl", "-w", "net.ipv4.conf."+target+".rp_filter=2")
	}
	return nil
}

func (a *App) cleanupPolicyRouting() {
	if runtime.GOOS != "linux" {
		return
	}
	_ = runCommand(context.Background(), "ip", "rule", "del", "table", "100")
	_ = runCommand(context.Background(), "ip", "route", "flush", "table", "100")
}

func (a *App) checkProxyHealth(ctx context.Context) map[string]any {
	if listener := a.checkProxyListener(ctx); listener["ok"] != true {
		return listener
	}
	if runtime.GOOS == "linux" && a.ovpn.Running() && a.cfg.LocalProxyOutboundDevice != "" {
		if _, err := os.Stat("/sys/class/net/" + a.cfg.LocalProxyOutboundDevice); err != nil {
			return map[string]any{"ok": false, "ip": "-", "latency_ms": 0, "error": "[ERR_ROUTE_DEV_NOT_FOUND] VPN tunnel device is not available"}
		}
	}
	proxyURL, err := url.Parse("http://" + net.JoinHostPort(a.connectProxyHost(), strconv.Itoa(a.ui.ProxyPort)))
	if err != nil {
		return map[string]any{"ok": false, "ip": "-", "latency_ms": 0, "error": err.Error()}
	}
	client := &http.Client{
		Timeout: 6 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}
	var lastErr string
	for _, target := range a.cfg.ProxyCheckURLs {
		started := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		body, readErr := readAllLimit(resp.Body, 256)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr.Error()
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return map[string]any{
				"ok":         true,
				"ip":         strings.TrimSpace(string(body)),
				"latency_ms": int(time.Since(started).Milliseconds()),
				"error":      "",
				"endpoint":   target,
			}
		}
		lastErr = resp.Status
	}
	if lastErr == "" {
		lastErr = "proxy egress check failed"
	}
	return map[string]any{"ok": false, "ip": "-", "latency_ms": 0, "error": lastErr}
}

func (a *App) checkProxyListener(ctx context.Context) map[string]any {
	address := net.JoinHostPort(a.connectProxyHost(), strconv.Itoa(a.ui.ProxyPort))
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return map[string]any{"ok": false, "ip": "-", "latency_ms": 0, "error": err.Error()}
	}
	_ = conn.Close()
	return map[string]any{"ok": true, "ip": "-", "latency_ms": 0, "error": ""}
}

func (a *App) connectProxyHost() string {
	host := a.cfg.LocalProxyHost
	if host == "" || host == "::" || host == "0.0.0.0" {
		return "127.0.0.1"
	}
	return host
}

func (a *App) proxyHealthLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.lastProxyHeartbeat = time.Now()
			if !a.ovpn.Running() || a.ovpn.NodeID() == "" {
				continue
			}
			result := a.checkProxyHealth(ctx)
			_ = a.store.UpdateState(map[string]any{
				"proxy_ok":         result["ok"],
				"proxy_ip":         result["ip"],
				"proxy_latency_ms": result["latency_ms"],
				"proxy_error":      result["error"],
			})
			if result["ok"] == true {
				a.proxyFailureCount = 0
				continue
			}
			a.proxyFailureCount++
			if a.proxyFailureCount >= 2 {
				a.handleProxyFailure(ctx, fmt.Sprint(result["error"]))
			}
		}
	}
}

func (a *App) handleProxyFailure(ctx context.Context, message string) {
	activeID := a.ovpn.NodeID()
	if activeID == "" {
		return
	}
	a.markNodeFailure(activeID, errors.New(message))
	if a.ui.RoutingMode == "fixed_ip" {
		_ = a.store.AppendLog("WARNING", "Proxy", "fixed IP node failed proxy health: "+message)
		return
	}
	a.ovpn.Stop()
	node, ok := a.selectRouteCandidate()
	if ok {
		_, _ = a.Connect(ctx, node.ID)
	}
}

func (a *App) activeLatencyLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.lastPingerHeartbeat = time.Now()
			if a.ovpn.Running() {
				_ = a.store.UpdateState(map[string]any{"active_node_latency": "running"})
			}
		}
	}
}

func runCommand(ctx context.Context, name string, args ...string) error {
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("%s: %w", msg, err)
		}
		return err
	}
	return nil
}
