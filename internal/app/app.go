package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/6Kmfi6HP/aimili-vpngate/internal/config"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/diagnostics"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/openvpn"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/proxy"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/state"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/vpngate"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/web"
)

type App struct {
	cfg    config.Config
	store  *state.Store
	ui     state.UIConfig
	ovpn   *openvpn.Manager
	nodes  []vpngate.Node
	mu     sync.Mutex
	opMu   sync.Mutex
	logger *log.Logger
}

func New(cfg config.Config) (*App, error) {
	store := state.NewStore(cfg.DataDir)
	if err := store.Ensure(); err != nil {
		return nil, err
	}
	if err := store.EnsureMigrationBackup(); err != nil {
		return nil, err
	}
	ui, err := store.LoadUIConfig(cfg.UIHost, cfg.UIPort, cfg.LocalProxyPort)
	if err != nil {
		return nil, err
	}
	logger := log.New(os.Stdout, "", log.LstdFlags)
	app := &App{cfg: cfg, store: store, ui: ui, logger: logger}
	app.ovpn = openvpn.NewManager(cfg, store.AuthFile(), logger)
	_ = state.ReadJSON(store.NodesFile(), &app.nodes)
	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	if err := a.store.EnsureWritable(); err != nil {
		return err
	}
	if err := a.ovpn.EnsureAuthFile(); err != nil {
		return err
	}
	a.updateInitialState()
	for _, check := range diagnostics.RuntimeChecks(a.cfg) {
		if !check.OK {
			_ = a.store.AppendLog("WARNING", "Diagnostics", check.Message)
		}
	}
	proxyAddr := net.JoinHostPort(a.cfg.LocalProxyHost, strconv.Itoa(a.ui.ProxyPort))
	proxyServer := &proxy.Server{
		Addr: proxyAddr,
		Dialer: proxy.Dialer{
			Timeout: 30 * time.Second,
			Device:  a.cfg.LocalProxyOutboundDevice,
		},
	}
	go func() {
		if err := proxyServer.ListenAndServe(ctx); err != nil {
			_ = a.store.AppendLog("ERROR", "Proxy", err.Error())
		}
	}()
	go a.maintainLoop(ctx)

	webAddr := net.JoinHostPort(a.ui.Host, strconv.Itoa(a.ui.Port))
	a.logger.Printf("UI: http://%s/", webAddr)
	a.logger.Printf("Proxy: http://%s", proxyAddr)
	return web.New(a).ListenAndServe(ctx, webAddr)
}

func (a *App) UIConfig() state.UIConfig {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ui
}

func (a *App) UpdateUIConfig(cfg state.UIConfig) error {
	if cfg.RoutingMode == "" {
		cfg.RoutingMode = "auto"
	}
	a.mu.Lock()
	a.ui = cfg
	a.mu.Unlock()
	return a.store.SaveUIConfig(cfg)
}

func (a *App) Nodes() ([]vpngate.Node, map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	nodes := make([]vpngate.Node, len(a.nodes))
	copy(nodes, a.nodes)
	activeID := a.ovpn.NodeID()
	for i := range nodes {
		nodes[i].Active = activeID != "" && nodes[i].ID == activeID
		nodes[i].ConfigText = ""
	}
	runtimeState, err := a.store.ReadState()
	if err != nil {
		return nil, nil, err
	}
	return nodes, runtimeState, nil
}

func (a *App) RefreshNodes(ctx context.Context, force bool) (string, error) {
	client := vpngate.Client{APIURL: a.cfg.APIURL, HTTP: &http.Client{Timeout: 25 * time.Second}}
	nodes, err := client.Fetch(ctx)
	if err != nil {
		_ = a.store.AppendLog("ERROR", "VPNGate", err.Error())
		_ = a.store.UpdateState(map[string]any{"last_fetch_status": "failed", "last_check_message": err.Error(), "is_connecting": false})
		return "", err
	}
	candidates := vpngate.SelectCandidates(nodes, a.cfg.MaxScanRows)
	checked := vpngate.CheckCandidates(ctx, candidates, a.cfg.TargetValidNodes, 3*time.Second)
	for i := range checked {
		path, err := openvpn.WriteConfig(a.store.ConfigDir(), checked[i])
		if err == nil {
			checked[i].ConfigFile = path
		}
	}
	a.mu.Lock()
	a.nodes = checked
	a.mu.Unlock()
	if err := state.WriteJSON(a.store.NodesFile(), checked, 0o644); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("loaded %d VPNGate candidates", len(checked))
	_ = a.store.UpdateState(map[string]any{"last_fetch_status": "ok", "last_check_message": msg, "is_connecting": false})
	_ = a.store.AppendLog("INFO", "VPNGate", msg)
	return msg, nil
}

func (a *App) Connect(ctx context.Context, id string) (string, error) {
	a.opMu.Lock()
	defer a.opMu.Unlock()
	a.mu.Lock()
	var selected *vpngate.Node
	for i := range a.nodes {
		if a.nodes[i].ID == id {
			node := a.nodes[i]
			selected = &node
			break
		}
	}
	a.mu.Unlock()
	if selected == nil {
		return "", fmt.Errorf("node %q not found", id)
	}
	if a.ui.RoutingMode == "fixed_ip" {
		cfg := a.UIConfig()
		cfg.FixedNodeID = selected.ID
		_ = a.UpdateUIConfig(cfg)
	}
	configPath := selected.ConfigFile
	if configPath == "" {
		path, err := openvpn.WriteConfig(a.store.ConfigDir(), *selected)
		if err != nil {
			return "", err
		}
		configPath = path
	}
	_ = a.store.UpdateState(map[string]any{"is_connecting": true, "active_openvpn_node_id": selected.ID, "last_check_message": "starting OpenVPN"})
	if err := a.ovpn.Start(ctx, *selected, configPath); err != nil {
		a.markNodeFailure(selected.ID, err)
		_ = a.store.UpdateState(map[string]any{"is_connecting": false, "last_check_message": err.Error()})
		return "", err
	}
	msg := "connected " + selected.ID
	_ = a.store.UpdateState(map[string]any{"is_connecting": false, "active_openvpn_node_id": selected.ID, "last_check_message": msg})
	return msg, nil
}

func (a *App) Disconnect() error {
	a.opMu.Lock()
	defer a.opMu.Unlock()
	a.ovpn.Stop()
	return a.store.UpdateState(map[string]any{"active_openvpn_node_id": "", "active_node_latency": "not connected", "last_check_message": "manual disconnect"})
}

func (a *App) TestNode(ctx context.Context, id string) (vpngate.Node, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, node := range a.nodes {
		if node.ID == id {
			checked := vpngate.CheckCandidates(ctx, []vpngate.Node{node}, 1, 3*time.Second)
			if len(checked) == 0 {
				return node, nil
			}
			return checked[0], nil
		}
	}
	return vpngate.Node{}, fmt.Errorf("node %q not found", id)
}

func (a *App) TestProxy(ctx context.Context) (map[string]any, error) {
	host := a.cfg.LocalProxyHost
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	address := net.JoinHostPort(host, strconv.Itoa(a.ui.ProxyPort))
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	_ = conn.Close()
	return map[string]any{"ok": true, "message": "proxy listener is reachable"}, nil
}

func (a *App) GatewayStatus() ([]map[string]any, error) {
	return []map[string]any{
		{"name": "Web management service", "status": "running", "details": fmt.Sprintf("%s:%d", a.ui.Host, a.ui.Port), "error": ""},
		{"name": "Local proxy gateway", "status": "running", "details": fmt.Sprintf("%s:%d", a.cfg.LocalProxyHost, a.ui.ProxyPort), "error": ""},
		{"name": "OpenVPN core", "status": status(a.ovpn.Running()), "details": a.ovpn.NodeID(), "error": ""},
	}, nil
}

func (a *App) Logs() ([]state.LogEntry, error) {
	return a.store.ReadLogs()
}

func (a *App) ConfigByName(name string) (string, bool) {
	path := filepath.Join(a.store.ConfigDir(), filepath.Base(name))
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

func (a *App) updateInitialState() {
	proxyURL := fmt.Sprintf("http://%s:%d", a.cfg.LocalProxyHost, a.ui.ProxyPort)
	if stringsContains(a.cfg.LocalProxyHost, ":") {
		proxyURL = fmt.Sprintf("http://[%s]:%d", a.cfg.LocalProxyHost, a.ui.ProxyPort)
	}
	_ = a.store.UpdateState(map[string]any{
		"api_url":                  a.cfg.APIURL,
		"target_valid_nodes":       a.cfg.TargetValidNodes,
		"fetch_interval_seconds":   a.cfg.FetchIntervalSeconds,
		"check_interval_seconds":   a.cfg.CheckIntervalSeconds,
		"local_proxy":              proxyURL,
		"active_openvpn_node_id":   "",
		"last_fetch_status":        "starting",
		"last_check_message":       "Go runtime started",
		"is_connecting":            false,
		"active_node_latency":      "not connected",
		"blacklisted_nodes":        0,
		"go_runtime_version":       a.cfg.Version,
		"invalid_backoff_seconds":  a.cfg.InvalidBackoffSeconds,
		"openvpn_timeout_seconds":  a.cfg.OpenVPNTestTimeoutSeconds,
		"container_mode":           a.cfg.ContainerMode,
		"management_secret_suffix": a.ui.SecretPath,
	})
}

func (a *App) maintainLoop(ctx context.Context) {
	a.maintainOnce(ctx)
	ticker := time.NewTicker(time.Duration(a.cfg.FetchIntervalSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.maintainOnce(ctx)
		}
	}
}

func (a *App) maintainOnce(ctx context.Context) {
	_, _ = a.RefreshNodes(ctx, false)
	if !a.cfg.AutoConnect {
		return
	}
	if a.ovpn.Running() {
		return
	}
	node, ok := a.selectRouteCandidate()
	if !ok {
		return
	}
	_, _ = a.Connect(ctx, node.ID)
}

func (a *App) selectRouteCandidate() (vpngate.Node, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cfg := a.ui
	var fallback *vpngate.Node
	for i := range a.nodes {
		node := a.nodes[i]
		if node.InvalidUntil > time.Now().Unix() {
			continue
		}
		if node.ProbeStatus != "ok" && fallback != nil {
			continue
		}
		switch cfg.RoutingMode {
		case "fixed_ip":
			if cfg.FixedNodeID != "" && node.ID == cfg.FixedNodeID {
				return node, true
			}
		case "fixed_region":
			if cfg.ForceCountry != "" && node.Country == cfg.ForceCountry {
				return node, true
			}
		default:
			if node.ProbeStatus == "ok" {
				return node, true
			}
		}
		if fallback == nil {
			copyNode := node
			fallback = &copyNode
		}
	}
	if cfg.RoutingMode == "auto" && fallback != nil {
		return *fallback, true
	}
	return vpngate.Node{}, false
}

func (a *App) markNodeFailure(id string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	until := time.Now().Add(time.Duration(a.cfg.InvalidBackoffSeconds) * time.Second).Unix()
	for i := range a.nodes {
		if a.nodes[i].ID == id {
			a.nodes[i].LastError = err.Error()
			a.nodes[i].ProbeStatus = "unavailable"
			a.nodes[i].InvalidUntil = until
			break
		}
	}
	_ = state.WriteJSON(a.store.NodesFile(), a.nodes, 0o644)
}

func status(ok bool) string {
	if ok {
		return "running"
	}
	return "stopped"
}

func stringsContains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s, substr)))
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
