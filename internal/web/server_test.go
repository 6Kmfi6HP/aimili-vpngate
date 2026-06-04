package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/6Kmfi6HP/aimili-vpngate/internal/state"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/vpngate"
)

type fakeBackend struct {
	cfg       state.UIConfig
	nodes     []vpngate.Node
	logs      []state.LogEntry
	testedIDs []string
}

func (f *fakeBackend) UIConfig() state.UIConfig { return f.cfg }
func (f *fakeBackend) UpdateUIConfig(cfg state.UIConfig) error {
	f.cfg = cfg
	return nil
}
func (f *fakeBackend) Nodes() ([]vpngate.Node, map[string]any, error) {
	return f.nodes, map[string]any{"status": "ok"}, nil
}
func (f *fakeBackend) RefreshNodes(context.Context, bool) (string, error) {
	return "refresh ok", nil
}
func (f *fakeBackend) Connect(context.Context, string) (string, error) {
	return "connect ok", nil
}
func (f *fakeBackend) Disconnect() error { return nil }
func (f *fakeBackend) TestNode(context.Context, string) (vpngate.Node, error) {
	return f.nodes[0], nil
}
func (f *fakeBackend) TestNodes(_ context.Context, ids []string) ([]vpngate.Node, error) {
	f.testedIDs = append([]string{}, ids...)
	return f.nodes, nil
}
func (f *fakeBackend) TestProxy(context.Context) (map[string]any, error) {
	return map[string]any{"ok": true}, nil
}
func (f *fakeBackend) GatewayStatus() ([]map[string]any, error) {
	return []map[string]any{{"name": "web", "status": "running"}}, nil
}
func (f *fakeBackend) Logs() ([]state.LogEntry, error) { return f.logs, nil }
func (f *fakeBackend) ConfigByName(string) (string, bool) {
	return "client\n", true
}

func TestSecretPathAndNodesAPI(t *testing.T) {
	backend := &fakeBackend{
		cfg:   state.UIConfig{SecretPath: "secret", Username: "admin"},
		nodes: []vpngate.Node{{ID: "node-1"}},
	}
	server := httptest.NewServer(New(backend))
	defer server.Close()

	resp, err := http.Get(server.URL + "/secret/api/nodes")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body["nodes"].([]any)) != 1 {
		t.Fatalf("nodes body = %#v", body)
	}
}

func TestUnauthorizedThenLogin(t *testing.T) {
	backend := &fakeBackend{cfg: state.UIConfig{SecretPath: "secret", Username: "admin", Password: "pass"}}
	srv := New(backend)
	server := httptest.NewServer(srv)
	defer server.Close()

	resp, err := http.Get(server.URL + "/secret/api/nodes")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth status = %d", resp.StatusCode)
	}

	login, err := http.Post(server.URL+"/secret/api/login", "application/json", strings.NewReader(`{"username":"admin","password":"pass"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = login.Body.Close()
	if login.StatusCode != http.StatusOK || len(login.Cookies()) == 0 {
		t.Fatalf("login failed: %d cookies=%d", login.StatusCode, len(login.Cookies()))
	}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/secret/api/nodes", nil)
	req.AddCookie(login.Cookies()[0])
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("auth status = %d", resp.StatusCode)
	}
}

func TestUpdateSettingsAndLogs(t *testing.T) {
	backend := &fakeBackend{
		cfg:  state.UIConfig{SecretPath: "secret", Username: "admin"},
		logs: []state.LogEntry{{Timestamp: "2026-01-02 03:04:05", Level: "INFO", Module: "Test", Message: "ok"}},
	}
	server := httptest.NewServer(New(backend))
	defer server.Close()

	resp, err := http.Post(server.URL+"/secret/api/update_settings", "application/json", strings.NewReader(`{"port":9999,"proxy_port":18888,"routing_mode":"fixed_region","force_country":"JP"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("update status = %d", resp.StatusCode)
	}
	var updateBody map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&updateBody); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if backend.cfg.Port != 9999 || backend.cfg.ProxyPort != 18888 || backend.cfg.ForceCountry != "JP" {
		t.Fatalf("settings not updated: %#v", backend.cfg)
	}
	if updateBody["restart_needed"] != true {
		t.Fatalf("listener change should require restart: %#v", updateBody)
	}
	resp, err = http.Get(server.URL + "/secret/api/logs")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logs status = %d", resp.StatusCode)
	}
}

func TestUpdateSettingsRejectsInvalidValues(t *testing.T) {
	backend := &fakeBackend{cfg: state.UIConfig{SecretPath: "secret", Username: "admin", Port: 8787, ProxyPort: 7928, RoutingMode: "auto"}}
	server := httptest.NewServer(New(backend))
	defer server.Close()

	resp, err := http.Post(server.URL+"/secret/api/update_settings", "application/json", strings.NewReader(`{"port":80,"proxy_port":80,"secret_path":"bad/path","routing_mode":"else"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	resp, err = http.Post(server.URL+"/secret/api/update_settings", "application/json", strings.NewReader(`{"secret_path":""}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty secret status = %d", resp.StatusCode)
	}
}

func TestUpdateRoutingDoesNotRequireRestart(t *testing.T) {
	backend := &fakeBackend{cfg: state.UIConfig{SecretPath: "secret", Username: "admin", Port: 8787, ProxyPort: 7928, RoutingMode: "auto"}}
	server := httptest.NewServer(New(backend))
	defer server.Close()

	resp, err := http.Post(server.URL+"/secret/api/update_settings", "application/json", strings.NewReader(`{"routing_mode":"fixed_region","force_country":"JP"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || body["restart_needed"] != false {
		t.Fatalf("unexpected response %d %#v", resp.StatusCode, body)
	}
	if backend.cfg.RoutingMode != "fixed_region" || backend.cfg.ForceCountry != "JP" {
		t.Fatalf("routing not saved: %#v", backend.cfg)
	}

	resp, err = http.Post(server.URL+"/secret/api/update_routing", "application/json", strings.NewReader(`{"routing_mode":"auto","force_country":""}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body = map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || body["restart_needed"] != false {
		t.Fatalf("unexpected update_routing response %d %#v", resp.StatusCode, body)
	}
}

func TestTestNodesCallsBackend(t *testing.T) {
	backend := &fakeBackend{
		cfg:   state.UIConfig{SecretPath: "secret", Username: "admin"},
		nodes: []vpngate.Node{{ID: "node-1"}, {ID: "node-2"}},
	}
	server := httptest.NewServer(New(backend))
	defer server.Close()

	resp, err := http.Post(server.URL+"/secret/api/test_nodes", "application/json", strings.NewReader(`{"ids":["node-1","node-2"]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if strings.Join(backend.testedIDs, ",") != "node-1,node-2" {
		t.Fatalf("tested ids = %#v", backend.testedIDs)
	}
}

func TestLogsEnsureDiagnosticCodes(t *testing.T) {
	backend := &fakeBackend{
		cfg:  state.UIConfig{SecretPath: "secret", Username: "admin"},
		logs: []state.LogEntry{{Timestamp: "2026-01-02 03:04:05", Level: "ERROR", Module: "VPN", Message: "[ERR_OVPN_NODE_UNREACHABLE] timeout"}},
	}
	server := httptest.NewServer(New(backend))
	defer server.Close()

	resp, err := http.Get(server.URL + "/secret/api/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var logsBody map[string][]state.LogEntry
	if err := json.NewDecoder(resp.Body).Decode(&logsBody); err != nil {
		t.Fatal(err)
	}
	if len(logsBody["logs"]) != 1 || !strings.Contains(logsBody["logs"][0].Message, "[2004] ERR_OVPN_NODE_UNREACHABLE") {
		t.Fatalf("log diagnostic code missing: %#v", logsBody)
	}
}

func TestIndexHTMLRendersStructuredGatewayAndLogs(t *testing.T) {
	for _, want := range []string{"Gateway Status", "id=\"services\"", "id=\"logRows\"", "services.innerHTML", "logRows.innerHTML", "runAction"} {
		if !strings.Contains(indexHTML, want) {
			t.Fatalf("indexHTML missing %q", want)
		}
	}
	for _, unwanted := range []string{"show(await api('gateway_status'))", "show(await api('logs'))"} {
		if strings.Contains(indexHTML, unwanted) {
			t.Fatalf("indexHTML still renders raw JSON path %q", unwanted)
		}
	}
}
