package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	diag "github.com/6Kmfi6HP/aimili-vpngate/internal/diagnostics"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/state"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/vpngate"
)

type Backend interface {
	UIConfig() state.UIConfig
	UpdateUIConfig(state.UIConfig) error
	Nodes() ([]vpngate.Node, map[string]any, error)
	RefreshNodes(context.Context, bool) (string, error)
	Connect(context.Context, string) (string, error)
	Disconnect() error
	TestNode(context.Context, string) (vpngate.Node, error)
	TestNodes(context.Context, []string) ([]vpngate.Node, error)
	TestProxy(context.Context) (map[string]any, error)
	GatewayStatus() ([]map[string]any, error)
	Logs() ([]state.LogEntry, error)
	ConfigByName(string) (string, bool)
}

type Server struct {
	Backend  Backend
	Sessions map[string]time.Time
	mu       sync.Mutex
}

func New(backend Backend) *Server {
	return &Server{Backend: backend, Sessions: map[string]time.Time{}}
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	server := &http.Server{Addr: addr, Handler: s}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	effective, ok := s.validatePath(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPost && (effective == "/api/login" || effective == "/api/logout") {
		s.handlePost(w, r, effective)
		return
	}
	if !s.authorized(r) {
		if effective == "/" || effective == "/index.html" {
			writeHTML(w, loginHTML)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "Unauthorized"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGet(w, r, effective)
	case http.MethodPost:
		s.handlePost(w, r, effective)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, path string) {
	switch {
	case path == "/" || path == "/index.html":
		writeHTML(w, indexHTML)
	case path == "/api/nodes":
		nodes, runtimeState, err := s.Backend.Nodes()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes, "state": runtimeState})
	case strings.HasPrefix(path, "/configs/"):
		name := filepath.Base(strings.TrimPrefix(path, "/configs/"))
		if content, ok := s.Backend.ConfigByName(name); ok {
			w.Header().Set("Content-Type", "application/x-openvpn-profile")
			_, _ = w.Write([]byte(content))
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	case path == "/api/gateway_status":
		services, err := s.Backend.GatewayStatus()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		services = withDiagnosticCodes(services)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "services": services})
	case path == "/api/logs":
		logs, err := s.Backend.Logs()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		logs = logsWithDiagnosticCodes(logs)
		writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request, path string) {
	if path == "/api/login" {
		s.login(w, r)
		return
	}
	if path == "/api/logout" {
		s.logout(w, r)
		return
	}
	var payload map[string]any
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload)
	switch path {
	case "/api/update_credentials":
		cfg := s.Backend.UIConfig()
		if value, ok := payload["username"].(string); ok && value != "" {
			cfg.Username = value
		}
		if value, ok := payload["password"].(string); ok {
			cfg.Password = value
		}
		if err := s.Backend.UpdateUIConfig(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "/api/update_settings":
		old := s.Backend.UIConfig()
		cfg := old
		if err := updateSettings(payload, &cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := s.Backend.UpdateUIConfig(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		restartNeeded := old.Port != cfg.Port || old.ProxyPort != cfg.ProxyPort || old.SecretPath != cfg.SecretPath
		message := "settings updated"
		if restartNeeded {
			message = "settings updated; restart required for listener changes"
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restart_needed": restartNeeded, "message": message})
	case "/api/update_routing":
		cfg := s.Backend.UIConfig()
		if err := updateRouting(payload, &cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := s.Backend.UpdateUIConfig(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restart_needed": false, "message": "routing updated"})
	case "/api/check":
		msg, err := s.Backend.RefreshNodes(r.Context(), true)
		writeResult(w, msg, err)
	case "/api/refresh_nodes":
		go func() {
			_, _ = s.Backend.RefreshNodes(context.Background(), false)
		}()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "node refresh started"})
	case "/api/test_nodes":
		ids := stringSlice(payload["ids"])
		nodes, err := s.Backend.TestNodes(r.Context(), ids)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "nodes": nodes})
	case "/api/disconnect":
		writeResult(w, "disconnected", s.Backend.Disconnect())
	case "/api/connect":
		msg, err := s.Backend.Connect(r.Context(), stringValue(payload["id"]))
		writeResult(w, msg, err)
	case "/api/test_node":
		node, err := s.Backend.TestNode(r.Context(), stringValue(payload["id"]))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "node": node})
	case "/api/test_proxy":
		result, err := s.Backend.TestProxy(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}

func (s *Server) validatePath(w http.ResponseWriter, r *http.Request) (string, bool) {
	secret := s.Backend.UIConfig().SecretPath
	if secret == "" {
		return r.URL.Path, true
	}
	if r.URL.Path == "/"+secret {
		http.Redirect(w, r, "/"+secret+"/", http.StatusFound)
		return "", false
	}
	prefix := "/" + secret + "/"
	if strings.HasPrefix(r.URL.Path, prefix) {
		return "/" + strings.TrimPrefix(r.URL.Path, prefix), true
	}
	http.NotFound(w, r)
	return "", false
}

func (s *Server) authorized(r *http.Request) bool {
	cfg := s.Backend.UIConfig()
	if cfg.Password == "" {
		return true
	}
	cookie, err := r.Cookie("session")
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.Sessions[cookie.Value]
	return ok && exp.After(time.Now())
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload)
	cfg := s.Backend.UIConfig()
	if cfg.Password == "" || (stringValue(payload["username"]) == cfg.Username && stringValue(payload["password"]) == cfg.Password) {
		token := time.Now().Format("20060102150405.000000000")
		s.mu.Lock()
		s.Sessions[token] = time.Now().Add(30 * 24 * time.Hour)
		s.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "session", Value: token, Path: "/" + cfg.SecretPath + "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 2592000})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "invalid username or password"})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session"); err == nil {
		s.mu.Lock()
		delete(s.Sessions, cookie.Value)
		s.mu.Unlock()
	}
	cfg := s.Backend.UIConfig()
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/" + cfg.SecretPath + "/", HttpOnly: true, MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeHTML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(body))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeResult(w http.ResponseWriter, message string, err error) {
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": message})
}

func withDiagnosticCodes(services []map[string]any) []map[string]any {
	out := make([]map[string]any, len(services))
	for i, service := range services {
		copyService := map[string]any{}
		for key, value := range service {
			copyService[key] = value
		}
		if errText, ok := copyService["error"].(string); ok {
			copyService["error"] = diag.EnsureCode(errText)
		}
		out[i] = copyService
	}
	return out
}

func logsWithDiagnosticCodes(logs []state.LogEntry) []state.LogEntry {
	out := make([]state.LogEntry, len(logs))
	for i, entry := range logs {
		entry.Message = diag.EnsureCode(entry.Message)
		out[i] = entry
	}
	return out
}

func updateSettings(payload map[string]any, cfg *state.UIConfig) error {
	if value, ok := payload["secret_path"].(string); ok {
		if !regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(value) {
			return fmt.Errorf("secret path must contain only letters and numbers")
		}
		cfg.SecretPath = value
	}
	if value, ok := payload["routing_mode"].(string); ok && value != "" {
		if !validRoutingMode(value) {
			return fmt.Errorf("invalid routing mode")
		}
		cfg.RoutingMode = value
	}
	if value, ok := payload["force_country"].(string); ok {
		cfg.ForceCountry = value
	}
	if value, ok := numberValue(payload["port"]); ok {
		if value < 1 || value > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
		cfg.Port = value
	}
	if value, ok := numberValue(payload["proxy_port"]); ok {
		if value < 1024 || value > 65535 {
			return fmt.Errorf("proxy port must be between 1024 and 65535")
		}
		cfg.ProxyPort = value
	}
	if cfg.Port == cfg.ProxyPort {
		return fmt.Errorf("management UI port and proxy port must differ")
	}
	if cfg.RoutingMode == "" {
		cfg.RoutingMode = "auto"
	}
	return nil
}

func updateRouting(payload map[string]any, cfg *state.UIConfig) error {
	if value, ok := payload["routing_mode"].(string); ok && value != "" {
		if !validRoutingMode(value) {
			return fmt.Errorf("invalid routing mode")
		}
		cfg.RoutingMode = value
	}
	if value, ok := payload["force_country"].(string); ok {
		cfg.ForceCountry = value
	}
	return nil
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func numberValue(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if s, ok := value.(string); ok && s != "" {
			return []string{s}
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func validRoutingMode(value string) bool {
	return value == "auto" || value == "fixed_ip" || value == "fixed_region"
}

const loginHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>AimiliVPN Login</title><style>body{font-family:sans-serif;margin:3rem;max-width:32rem}input,button{font:inherit;padding:.7rem;margin:.3rem 0;width:100%}</style></head>
<body><h1>AimiliVPN</h1><form id="f"><input name="username" placeholder="admin" autocomplete="username"><input name="password" type="password" placeholder="password" autocomplete="current-password"><button>Login</button></form><p id="m"></p><script>
f.onsubmit=async e=>{e.preventDefault();const d=Object.fromEntries(new FormData(f));const r=await fetch('./api/login',{method:'POST',body:JSON.stringify(d)});if(r.ok) location.reload(); else m.textContent='Login failed';};
</script></body></html>`

const legacyIndexHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>AimiliVPN</title><style>
body{font-family:system-ui,sans-serif;margin:0;background:#101418;color:#eef2f3}header,section{padding:18px 24px}button,input,select{font:inherit;padding:.48rem;margin:.16rem}button{cursor:pointer}table{border-collapse:collapse;width:100%;font-size:14px}td,th{border-bottom:1px solid #303940;padding:.5rem;text-align:left;vertical-align:top}section{border-top:1px solid #303940}.row{display:flex;gap:10px;flex-wrap:wrap;align-items:center}.muted{color:#aab4bb}.ok{color:#65d480}.bad{color:#ff8080}.warn{color:#ffd36a}pre{white-space:pre-wrap;background:#182027;padding:12px;max-height:160px;overflow:auto}.feedback{min-height:1.5rem}
</style></head><body>
<header><h1>AimiliVPN</h1><div id="summary" class="muted">Loading...</div></header>
<section><div class="row"><button onclick="refreshNodes()">Refresh</button><button onclick="testSelected()">Test selected</button><button onclick="testProxy()">Test proxy</button><button onclick="loadStatus()">Gateway</button><button onclick="loadLogs()">Logs</button></div><div id="feedback" class="feedback muted"></div></section>
<section><h2>Nodes</h2><table><thead><tr><th></th><th>ID</th><th>Country</th><th>Remote</th><th>Latency</th><th>Status</th><th>Actions</th></tr></thead><tbody id="nodes"></tbody></table></section>
<section><h2>Settings</h2><div class="row"><input id="uiPort" type="number" placeholder="UI port"><input id="proxyPort" type="number" placeholder="Proxy port"><input id="secretPath" placeholder="Secret path"><select id="routeMode"><option value="auto">auto</option><option value="fixed_region">fixed_region</option><option value="fixed_ip">fixed_ip</option></select><input id="forceCountry" placeholder="Country"><button onclick="saveSettings()">Save settings</button><button onclick="saveRouting()">Save routing</button></div><div class="row"><input id="username" placeholder="Username"><input id="password" type="password" placeholder="Password"><button onclick="saveCredentials()">Update credentials</button></div></section>
<section><h2>Gateway Status</h2><table><thead><tr><th>Service</th><th>Status</th><th>Details</th><th>Error</th></tr></thead><tbody id="services"></tbody></table></section>
<section><h2>Logs</h2><table><thead><tr><th>Time</th><th>Level</th><th>Module</th><th>Message</th></tr></thead><tbody id="logRows"></tbody></table></section>
<section><h2>Output</h2><pre id="out">Ready.</pre></section>
<script>
let current={nodes:[],state:{}};
async function api(path,opts={}){const r=await fetch('./api/'+path,{headers:{'Content-Type':'application/json'},...opts});let j={};try{j=await r.json()}catch{}if(!r.ok||j.ok===false) throw new Error(j.error||j.message||r.statusText);return j}
function esc(s){return String(s??'').replace(/[&<>"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]))}
async function load(){current=await api('nodes');render();await loadStatus(false);await loadLogs(false)}
function render(){const s=current.state||{};summary.textContent='Active: '+(s.active_openvpn_node_id||'none')+' | Proxy: '+(s.proxy_ok?'ok':'not ready')+' | '+(s.last_check_message||'');uiPort.value=s.port||'';proxyPort.value=s.proxy_port||'';secretPath.value=s.secret_path||'';routeMode.value=s.routing_mode||'auto';forceCountry.value=s.force_country||'';username.value=s.username||'';nodes.innerHTML=(current.nodes||[]).map(n=>'<tr><td><input type="checkbox" value="'+esc(n.id)+'"></td><td>'+esc(n.id)+(n.active?' <span class="ok">active</span>':'')+'</td><td>'+esc(n.country||n.country_short)+'</td><td>'+esc(n.remote_host)+':'+esc(n.remote_port)+' '+esc(n.proto||n.remote_proto)+'</td><td>'+esc(n.latency_ms||n.ping||'')+'</td><td class="'+(n.probe_status==='available'||n.probe_status==='ok'?'ok':'bad')+'">'+esc(n.probe_status||'not_checked')+'</td><td><button onclick="connectNode(\''+esc(n.id)+'\')">Connect</button><button onclick="testNode(\''+esc(n.id)+'\')">Test</button><button onclick="disconnectNode()">Disconnect</button></td></tr>').join('')}
function show(message,bad=false){feedback.textContent=message||'';feedback.className='feedback '+(bad?'bad':'ok');out.textContent=message||'Ready.'}
async function runAction(label,fn,reload=true){try{const r=await fn();const msg=r.message||(r.restart_needed?'Restart required for listener changes':label+' complete');show(msg+(r.nodes?' ('+r.nodes.length+' nodes)':''));if(reload) await load();return r}catch(e){show(e.message||String(e),true)}}
async function refreshNodes(){await runAction('Refresh',()=>api('refresh_nodes',{method:'POST'}));setTimeout(load,600)}
async function testSelected(){const ids=[...document.querySelectorAll('tbody input:checked')].map(x=>x.value);await runAction('Batch test',()=>api('test_nodes',{method:'POST',body:JSON.stringify({ids})}))}
async function testNode(id){await runAction('Node test',()=>api('test_node',{method:'POST',body:JSON.stringify({id})}))}
async function connectNode(id){await runAction('Connect',()=>api('connect',{method:'POST',body:JSON.stringify({id})}))}
async function disconnectNode(){await runAction('Disconnect',()=>api('disconnect',{method:'POST'}))}
async function testProxy(){await runAction('Proxy test',()=>api('test_proxy',{method:'POST'}))}
async function loadStatus(notify=true){try{const r=await api('gateway_status');services.innerHTML=(r.services||[]).map(s=>'<tr><td>'+esc(s.name)+'</td><td class="'+(s.status==='running'?'ok':s.status==='starting'?'warn':'bad')+'">'+esc(s.status)+'</td><td>'+esc(s.details||'')+'</td><td class="bad">'+esc(s.error||'')+'</td></tr>').join('');if(notify) show('Gateway status refreshed')}catch(e){if(notify) show(e.message,true)}}
async function loadLogs(notify=true){try{const r=await api('logs');logRows.innerHTML=(r.logs||[]).slice(-120).reverse().map(l=>'<tr><td>'+esc(l.timestamp||l.time)+'</td><td>'+esc(l.level)+'</td><td>'+esc(l.module)+'</td><td>'+esc(l.message)+'</td></tr>').join('');if(!logRows.innerHTML) logRows.innerHTML='<tr><td colspan="4" class="muted">No logs yet.</td></tr>';if(notify) show('Logs refreshed')}catch(e){if(notify) show(e.message,true)}}
async function saveSettings(){await runAction('Settings',()=>api('update_settings',{method:'POST',body:JSON.stringify({port:Number(uiPort.value),proxy_port:Number(proxyPort.value),secret_path:secretPath.value,routing_mode:routeMode.value,force_country:forceCountry.value})}))}
async function saveRouting(){await runAction('Routing',()=>api('update_routing',{method:'POST',body:JSON.stringify({routing_mode:routeMode.value,force_country:forceCountry.value})}))}
async function saveCredentials(){await runAction('Credentials',()=>api('update_credentials',{method:'POST',body:JSON.stringify({username:username.value,password:password.value})}),false)}
load().catch(e=>show(e.message));
</script></body></html>`

const indexHTML = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>AimiliVPN</title>
<style>
:root{color-scheme:dark;--bg:#0e1116;--panel:#151a21;--panel2:#1b222b;--line:#2b3440;--text:#eef3f7;--muted:#9aa8b5;--good:#31d07d;--warn:#f4c542;--bad:#ff6b6b;--accent:#64a7ff}
*{box-sizing:border-box}body{font-family:system-ui,-apple-system,Segoe UI,sans-serif;margin:0;background:var(--bg);color:var(--text);font-size:14px}header{display:flex;justify-content:space-between;gap:16px;align-items:center;padding:18px 24px;border-bottom:1px solid var(--line);background:#111720;position:sticky;top:0;z-index:5}h1,h2,h3{margin:0}h1{font-size:22px}h2{font-size:16px}button,input,select{font:inherit;border-radius:6px;border:1px solid var(--line);background:#10161d;color:var(--text);padding:8px 10px}button{cursor:pointer;background:#1f2a36}button:hover:not(:disabled){border-color:var(--accent)}button:disabled{opacity:.45;cursor:not-allowed}a{color:var(--accent);text-decoration:none}main{padding:18px 24px;display:grid;gap:16px}.section{border-top:1px solid var(--line);padding-top:16px}.toolbar,.row,.header-links{display:flex;gap:10px;flex-wrap:wrap;align-items:center}.header-links{justify-content:flex-end}.muted{color:var(--muted)}.feedback{min-height:22px}.ok,.running{color:var(--good)}.bad,.stopped{color:var(--bad)}.warn,.starting{color:var(--warn)}.card{background:var(--panel);border:1px solid var(--line);border-radius:8px;padding:14px}.active-card{display:flex;justify-content:space-between;gap:14px;align-items:center}.active-title{display:flex;gap:8px;align-items:center;margin-bottom:6px}.badge{display:inline-flex;gap:6px;align-items:center;border:1px solid var(--line);border-radius:999px;padding:2px 8px;font-size:12px;color:var(--muted)}.available{color:var(--good);border-color:#265c43}.unavailable{color:var(--bad);border-color:#694040}.not_checked{color:var(--muted)}.connecting{color:var(--warn);border-color:#65562a}.pulse{width:7px;height:7px;border-radius:50%;background:currentColor;animation:pulse 1s infinite}@keyframes pulse{50%{opacity:.35}}@keyframes spin{to{transform:rotate(360deg)}}.spinner{display:inline-block;animation:spin 1s linear infinite}.grid{display:grid;gap:12px;grid-template-columns:repeat(4,minmax(0,1fr))}.stat{background:var(--panel2);border:1px solid var(--line);border-radius:8px;padding:12px}.stat strong{display:block;font-size:20px;margin-top:4px}table{border-collapse:collapse;width:100%;font-size:13px}th,td{border-bottom:1px solid var(--line);padding:9px 8px;text-align:left;vertical-align:middle}th{color:var(--muted);font-weight:600;background:#111820}tbody tr.active-row{background:#13281f}.mono{font-family:ui-monospace,SFMono-Regular,Consolas,monospace}.latency-good{color:var(--good)}.latency-medium{color:var(--warn)}.latency-poor{color:var(--bad)}.pagination{display:flex;justify-content:space-between;gap:12px;align-items:center;padding:12px 0;flex-wrap:wrap}.table-wrap{overflow:auto;border:1px solid var(--line);border-radius:8px}.modal{position:fixed;inset:0;background:rgba(3,6,10,.72);display:none;align-items:center;justify-content:center;padding:22px;z-index:20}.modal-body{background:#111720;border:1px solid var(--line);border-radius:8px;width:min(960px,100%);max-height:86vh;overflow:auto;padding:16px}.modal-head{display:flex;justify-content:space-between;gap:12px;align-items:center;margin-bottom:12px}.log-tools{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:10px}.service-card{background:var(--panel);border:1px solid var(--line);border-radius:8px;padding:12px;margin-bottom:10px}.service-card-head{display:flex;justify-content:space-between;gap:10px;align-items:center}.routing-note{max-width:760px;line-height:1.45}.danger{background:#3a1f25;border-color:#6e303a}.primary{background:#19324d;border-color:#315d8a}@media(max-width:900px){header{align-items:flex-start;flex-direction:column}.grid{grid-template-columns:repeat(2,minmax(0,1fr))}main{padding:14px}.active-card{align-items:flex-start;flex-direction:column}}@media(max-width:560px){.grid{grid-template-columns:1fr}.toolbar input,.toolbar select{width:100%}.table-wrap{font-size:12px}}
</style>
</head>
<body>
<header>
  <div>
    <h1>AimiliVPN</h1>
    <div id="summary" class="muted">Loading...</div>
  </div>
  <nav class="header-links">
    <a href="https://github.com/baoweise-bot/aimili-vpngate" target="_blank" rel="noopener">GitHub</a>
    <a href="https://t.me/arestemple" target="_blank" rel="noopener">Telegram</a>
    <button onclick="openGatewayModal()">Gateway</button>
    <button onclick="openLogsModal()">Logs</button>
  </nav>
</header>
<main>
  <div id="activeConnection" class="card"></div>

  <section class="section">
    <div class="grid">
      <div class="stat"><span class="muted">Total nodes</span><strong id="total">0</strong></div>
      <div class="stat"><span class="muted">Target valid</span><strong id="target">0</strong></div>
      <div class="stat"><span class="muted">Active</span><strong id="active">0</strong></div>
      <div class="stat"><span class="muted">Proxy</span><strong id="proxyState">-</strong></div>
    </div>
  </section>

  <section class="section">
    <div class="toolbar">
      <input id="search" placeholder="Search country, location, IP, ASN, ISP">
      <select id="country_filter"><option value="">All countries</option></select>
      <button id="refresh" class="primary" onclick="refreshNodes()">Refresh</button>
      <button id="btn_batch_test" onclick="testSelected()">Test selected</button>
      <button onclick="testProxy()">Test proxy</button>
    </div>
    <div id="feedback" class="feedback muted"></div>
  </section>

  <section class="section">
    <h2>Nodes</h2>
    <div class="table-wrap">
      <table>
        <thead><tr><th></th><th>Status</th><th>Latency</th><th>Remote</th><th>Country</th><th>Location</th><th>ASN</th><th>ISP / Owner</th><th>Quality</th><th>IP Type</th><th>Score</th><th>Actions</th></tr></thead>
        <tbody id="nodes"></tbody>
      </table>
    </div>
    <div class="pagination">
      <div class="muted">Showing <span id="page_start">0</span>-<span id="page_end">0</span> of <span id="filtered_count">0</span></div>
      <div class="row">
        <button id="btn_first_page" onclick="goPage('first')">First</button>
        <button id="btn_prev_page" onclick="goPage('prev')">Prev</button>
        <span class="muted">Page <strong id="current_page_val">1</strong> / <strong id="total_pages_val">1</strong></span>
        <button id="btn_next_page" onclick="goPage('next')">Next</button>
        <button id="btn_last_page" onclick="goPage('last')">Last</button>
      </div>
    </div>
  </section>

  <section class="section">
    <h2>Settings</h2>
    <div class="row">
      <input id="uiPort" type="number" placeholder="UI port">
      <input id="proxyPort" type="number" placeholder="Proxy port">
      <input id="secretPath" placeholder="Secret path">
      <select id="routeMode" onchange="handleRoutingModeChange(this.value)"><option value="auto">auto</option><option value="fixed_region">fixed_region</option><option value="fixed_ip">fixed_ip</option></select>
      <input id="forceCountry" placeholder="Country code/name">
      <button onclick="saveSettings()">Save settings</button>
      <button onclick="saveRouting()">Save routing</button>
    </div>
    <div id="routingDescription" class="routing-note muted"></div>
    <div class="row" style="margin-top:10px">
      <input id="username" placeholder="Username">
      <input id="password" type="password" placeholder="Password">
      <button onclick="saveCredentials()">Update credentials</button>
    </div>
  </section>

  <section class="section">
    <h2>Output</h2>
    <pre id="out" class="card">Ready.</pre>
  </section>
</main>

<div id="gatewayModal" class="modal" onclick="modalBackdrop(event,'gatewayModal')">
  <div class="modal-body">
    <div class="modal-head"><h2>Gateway Status</h2><button onclick="closeGatewayModal()">Close</button></div>
    <div id="serviceCards"></div>
    <table><thead><tr><th>Service</th><th>Status</th><th>Details</th><th>Error</th></tr></thead><tbody id="services"></tbody></table>
  </div>
</div>

<div id="logsModal" class="modal" onclick="modalBackdrop(event,'logsModal')">
  <div class="modal-body">
    <div class="modal-head"><h2>Logs</h2><button onclick="closeLogsModal()">Close</button></div>
    <div class="log-tools">
      <select id="logFilter" onchange="filterAndRenderLogs()"><option value="all">All</option><option value="proxy">Proxy</option><option value="vpn">VPN</option><option value="system">System</option></select>
      <button onclick="copyLogContent()">Copy</button>
      <button onclick="exportLogContent()">Export .txt</button>
    </div>
    <table><thead><tr><th>Time</th><th>Level</th><th>Module</th><th>Message</th></tr></thead><tbody id="logRows"></tbody></table>
  </div>
</div>

<script>
let current={nodes:[],state:{}};
let currentPage=1;
const pageSize=11;
let currentPageNodes=[];
let connectionPollInterval=null;
let gatewayPollInterval=null;
let logsPollInterval=null;
let rawLogsCache=[];
let lastInteraction=Date.now();
let refreshInFlight=false;

const $=id=>document.getElementById(id);
function esc(s){return String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#039;'}[c]))}
async function api(path,opts={}){const r=await fetch('./api/'+path,{headers:{'Content-Type':'application/json'},...opts});let j={};try{j=await r.json()}catch{}if(!r.ok||j.ok===false) throw new Error(j.error||j.message||r.statusText);return j}
function show(message,bad=false){feedback.textContent=message||'';feedback.className='feedback '+(bad?'bad':'ok');out.textContent=message||'Ready.'}
async function runAction(label,fn,reload=true){try{const r=await fn();const msg=r.message||(r.restart_needed?'Restart required for listener changes':label+' complete');show(msg+(r.nodes?' ('+r.nodes.length+' nodes)':''));if(reload) await load(false);return r}catch(e){show(e.message||String(e),true)}}

const translateQuality=q=>({"normal":"普通","proxy":"代理","datacenter":"数据中心","mobile":"移动端"}[q]||q||"-");
const translateIpType=t=>({"residential":"住宅 IP","hosting":"机房 IP","mobile":"移动网","proxy":"代理 IP"}[t]||t||"-");
function translateCountry(c){const dict={"Japan":"日本","Korea Republic of":"韩国","Korea":"韩国","Republic of Korea":"韩国","Thailand":"泰国","United States":"美国","United Kingdom":"英国","Russian Federation":"俄罗斯","Russian":"俄罗斯","Viet Nam":"越南","Vietnam":"越南","China":"中国","Taiwan":"台湾","Taiwan Province of China":"台湾","Hong Kong":"香港","Singapore":"新加坡","Malaysia":"马来西亚","Indonesia":"印度尼西亚","India":"印度","Philippines":"菲律宾","Australia":"澳大利亚","New Zealand":"新西兰","Canada":"加拿大","Ukraine":"乌克兰","France":"法国","Germany":"德国","Netherlands":"荷兰","Sweden":"瑞典","Norway":"挪威","Spain":"西班牙","Turkey":"土耳其","South Africa":"南非","Brazil":"巴西","Argentina":"阿根廷","Chile":"智利","Mexico":"墨西哥","Egypt":"埃及","Romania":"罗马尼亚","Poland":"波兰","Kazakhstan":"哈萨克斯坦","Georgia":"格鲁吉亚","Mongolia":"蒙古","Saudi Arabia":"沙特阿拉伯","Iran":"伊朗","Iraq":"伊拉克","Colombia":"哥伦比亚","Cambodia":"柬埔寨","Ireland":"爱尔兰","Italy":"意大利","Switzerland":"瑞士","Belgium":"比利时","Austria":"奥地利","Denmark":"丹麦","Finland":"芬兰","Portugal":"葡萄牙","Greece":"希腊","Czech Republic":"捷克","Hungary":"匈牙利","Israel":"以色列","United Arab Emirates":"阿联酋","UAE":"阿联酋","Macao":"澳门","Macau":"澳门","Iceland":"冰岛","Luxembourg":"卢森堡","Peru":"秘鲁","Pakistan":"巴基斯坦","Bangladesh":"孟加拉国","Myanmar":"缅甸","Laos":"老挝","Nepal":"尼泊尔","Sri Lanka":"斯里兰卡","Serbia":"塞尔维亚","Croatia":"克罗地亚","Slovakia":"斯洛伐克","Slovenia":"斯洛文尼亚","Bulgaria":"保加利亚","Lithuania":"立陶宛","Latvia":"拉脱维亚","Estonia":"爱沙尼亚"};return dict[c]||c||"-"}
const translateStatus=s=>({"available":"可用","ok":"可用","unavailable":"不可用","not_checked":"待检测"}[s]||s||"待检测");
function getLatencyClass(ms){ms=Number(ms||0);if(!ms)return '';if(ms<50)return 'latency-good';if(ms<150)return 'latency-medium';return 'latency-poor'}
function latencyText(ms){return ms?'<span class="'+getLatencyClass(ms)+'">'+esc(ms)+' ms</span>':'-'}

function stableSortNodes(){current.nodes=(current.nodes||[]).slice().sort((a,b)=>{const d=(b.score||0)-(a.score||0);if(d!==0)return d;return String(a.id||'').localeCompare(String(b.id||''))})}
function updateCountryFilter(){const select=$('country_filter');const selected=select.value;const counts={};(current.nodes||[]).forEach(n=>{const c=n.country||n.country_long||n.country_short;if(c)counts[c]=(counts[c]||0)+1});const countries=Object.keys(counts).sort((a,b)=>translateCountry(a).localeCompare(translateCountry(b)));select.innerHTML='<option value="">All countries</option>'+countries.map(c=>'<option value="'+esc(c)+'">'+esc(translateCountry(c))+' ('+counts[c]+')</option>').join('');select.value=countries.includes(selected)?selected:''}
function getFilteredNodes(){const q=$('search').value.trim().toLowerCase();const country=$('country_filter').value;return (current.nodes||[]).filter(n=>{if(!n)return false;const nodeCountry=n.country||n.country_long||n.country_short||'';if(country&&nodeCountry!==country)return false;const hay=[nodeCountry,translateCountry(nodeCountry),n.country_short,n.ip,n.remote_host,n.host_name,n.hostname,n.location,n.asn,n.as_name,n.owner,translateQuality(n.quality),translateIpType(n.ip_type),n.proto,n.remote_proto].join(' ').toLowerCase();return hay.includes(q)})}

function renderActiveConnection(){const s=current.state||{};const activeId=s.active_openvpn_node_id;const active=(current.nodes||[]).find(n=>n.active||n.id===activeId);if(s.is_connecting&&!active){activeConnection.innerHTML='<div class="active-card"><div><div class="active-title"><span class="badge connecting"><span class="pulse"></span>Connecting</span><strong>'+esc(s.active_node_latency||'Starting')+'</strong></div><div class="muted">'+esc(s.last_check_message||'Opening VPN tunnel')+'</div></div><button class="danger" onclick="disconnectNode()">Disconnect</button></div>';return}if(active){const country=translateCountry(active.country||active.country_long||active.country_short);activeConnection.innerHTML='<div class="active-card"><div><div class="active-title"><span class="badge available"><span class="pulse"></span>Active</span><strong>'+esc(country)+' node</strong></div><div class="mono">'+esc(active.ip||active.remote_host)+':'+esc(active.remote_port||'')+'</div><div class="muted">Latency '+latencyText(active.latency_ms||active.ping)+' | '+esc(active.location||country)+' | '+esc(active.owner||active.as_name||'-')+' | '+esc(translateIpType(active.ip_type))+'</div></div><button class="danger" onclick="disconnectNode()">Disconnect</button></div>';return}activeConnection.innerHTML='<div class="active-card"><div><div class="active-title"><span class="badge unavailable">Disconnected</span><strong>No active VPN node</strong></div><div class="muted">Choose a node below to connect.</div></div></div>'}

function render(){const s=current.state||{};renderActiveConnection();summary.textContent='Active: '+(s.active_openvpn_node_id||'none')+' | Proxy: '+(s.proxy_ok?'ok':'not ready')+' | '+(s.last_check_message||'');total.textContent=(current.nodes||[]).length;target.textContent=s.target_valid_nodes||0;active.textContent=s.active_openvpn_node_id?1:0;proxyState.textContent=s.proxy_ok?'ok':'not ready';proxyState.className=s.proxy_ok?'ok':'warn';uiPort.value=s.port||'';proxyPort.value=s.proxy_port||'';secretPath.value=s.secret_path||'';routeMode.value=s.routing_mode||'auto';forceCountry.value=s.force_country||'';username.value=s.username||'';handleRoutingModeChange(routeMode.value,false);
const shown=getFilteredNodes();const pages=Math.max(1,Math.ceil(shown.length/pageSize));if(currentPage>pages)currentPage=pages;if(currentPage<1)currentPage=1;const start=(currentPage-1)*pageSize;const end=Math.min(start+pageSize,shown.length);currentPageNodes=shown.slice(start,end);
nodes.innerHTML=currentPageNodes.length?currentPageNodes.map(n=>{const activeRow=n.active||n.id===s.active_openvpn_node_id;const status=activeRow?'available':(n.probe_status||'not_checked');const remote=(n.ip||n.remote_host||'-')+((n.remote_port)?':'+n.remote_port:'');return '<tr '+(activeRow?'class="active-row"':'')+'><td><input type="checkbox" value="'+esc(n.id)+'"></td><td><span class="badge '+esc(status)+'">'+(activeRow?'<span class="pulse"></span>已连接':esc(translateStatus(status)))+'</span></td><td>'+latencyText(n.latency_ms||n.ping)+'</td><td class="mono">'+esc(remote)+' '+esc(n.proto||n.remote_proto||'')+'</td><td>'+esc(translateCountry(n.country||n.country_long||n.country_short))+'</td><td>'+esc(n.location||'-')+'</td><td class="mono">'+esc(n.asn||'-')+'</td><td>'+esc(n.owner||n.as_name||'-')+'</td><td>'+esc(translateQuality(n.quality))+'</td><td>'+esc(translateIpType(n.ip_type))+'</td><td>'+esc(n.score||0)+'</td><td><button data-action="test" data-id="'+esc(n.id)+'">Test</button> <button data-action="connect" data-id="'+esc(n.id)+'" '+(status==='unavailable'||s.is_connecting?'disabled':'')+'>Connect</button></td></tr>'}).join(''):'<tr><td colspan="12" class="muted" style="text-align:center;padding:28px">No nodes match the current filter.</td></tr>';
page_start.textContent=shown.length?start+1:0;page_end.textContent=end;filtered_count.textContent=shown.length;current_page_val.textContent=currentPage;total_pages_val.textContent=pages;btn_first_page.disabled=currentPage===1;btn_prev_page.disabled=currentPage===1;btn_next_page.disabled=currentPage===pages;btn_last_page.disabled=currentPage===pages}

function goPage(which){const pages=Math.max(1,Math.ceil(getFilteredNodes().length/pageSize));if(which==='first')currentPage=1;if(which==='prev')currentPage=Math.max(1,currentPage-1);if(which==='next')currentPage=Math.min(pages,currentPage+1);if(which==='last')currentPage=pages;render()}
nodes.addEventListener('click',e=>{const b=e.target.closest('button[data-action]');if(!b)return;const id=b.dataset.id;if(b.dataset.action==='connect')connectNode(id);if(b.dataset.action==='test')testNode(id)});
search.oninput=()=>{currentPage=1;render()};country_filter.onchange=()=>{currentPage=1;render()};

async function load(silent=true){refreshInFlight=true;try{current=await api('nodes');stableSortNodes();updateCountryFilter();render();if(current.state&&current.state.is_connecting)startConnectionPolling();if(!silent)show('Data refreshed')}catch(e){if(!silent)show(e.message,true)}finally{refreshInFlight=false}}
async function refreshNodes(){await runAction('Refresh',()=>api('refresh_nodes',{method:'POST'}),false);setTimeout(()=>load(false),700)}
async function testSelected(){const ids=[...document.querySelectorAll('#nodes input:checked')].map(x=>x.value);const chosen=ids.length?ids:currentPageNodes.map(n=>n.id);await runAction('Batch test',()=>api('test_nodes',{method:'POST',body:JSON.stringify({ids:chosen})}))}
async function testNode(id){await runAction('Node test',()=>api('test_node',{method:'POST',body:JSON.stringify({id})}))}
async function connectNode(id){current.state=current.state||{};current.state.is_connecting=true;current.state.active_openvpn_node_id=id;current.state.active_node_latency='connecting';current.state.last_check_message='connection request sent';render();startConnectionPolling();await runAction('Connect',()=>api('connect',{method:'POST',body:JSON.stringify({id})}),false)}
async function disconnectNode(){if(!confirm('确定要断开当前 VPN 连接吗？'))return;await runAction('Disconnect',()=>api('disconnect',{method:'POST'}));try{await api('test_proxy',{method:'POST'})}catch{}}
async function testProxy(){await runAction('Proxy test',()=>api('test_proxy',{method:'POST'}))}

function startConnectionPolling(){if(connectionPollInterval)clearInterval(connectionPollInterval);const started=Date.now();connectionPollInterval=setInterval(async()=>{try{current=await api('nodes');stableSortNodes();updateCountryFilter();render();if(!current.state.is_connecting||Date.now()-started>90000){clearInterval(connectionPollInterval);connectionPollInterval=null;try{await api('test_proxy',{method:'POST'})}catch{}await load(true)}}catch(e){clearInterval(connectionPollInterval);connectionPollInterval=null;show(e.message,true)}},1000)}

async function saveSettings(){await runAction('Settings',()=>api('update_settings',{method:'POST',body:JSON.stringify({port:Number(uiPort.value),proxy_port:Number(proxyPort.value),secret_path:secretPath.value,routing_mode:routeMode.value,force_country:forceCountry.value})}))}
async function saveRouting(){await runAction('Routing',()=>api('update_routing',{method:'POST',body:JSON.stringify({routing_mode:routeMode.value,force_country:forceCountry.value})}))}
async function saveCredentials(){await runAction('Credentials',()=>api('update_credentials',{method:'POST',body:JSON.stringify({username:username.value,password:password.value})}),false)}
function handleRoutingModeChange(mode,rerender){let text='自动配置：系统测试并选择最佳节点，当前节点失效时自动切换。';if(mode==='fixed_region')text='固定地区：仅连接指定国家或地区的节点；该地区没有可用节点时不会切换到其他国家。';if(mode==='fixed_ip')text='固定 IP：锁定当前或指定节点，节点故障时不会漂移到其他 IP。';routingDescription.textContent=text;if(rerender!==false)render()}

function modalBackdrop(event,id){if(event.target&&event.target.id===id){if(id==='gatewayModal')closeGatewayModal();if(id==='logsModal')closeLogsModal()}}
function openGatewayModal(){gatewayModal.style.display='flex';loadStatus(false);if(gatewayPollInterval)clearInterval(gatewayPollInterval);gatewayPollInterval=setInterval(()=>loadStatus(false),3000)}
function closeGatewayModal(){gatewayModal.style.display='none';if(gatewayPollInterval){clearInterval(gatewayPollInterval);gatewayPollInterval=null}}
async function loadStatus(notify=true){try{const r=await api('gateway_status');renderGatewayServices(r.services||[]);if(notify)show('Gateway status refreshed')}catch(e){if(notify)show(e.message,true)}}
function renderGatewayServices(list){serviceCards.innerHTML=list.map(s=>'<div class="service-card"><div class="service-card-head"><strong>'+esc(s.name)+'</strong><span class="badge '+esc(s.status||'stopped')+'">'+esc(s.status||'unknown')+'</span></div><div class="muted">'+esc(s.details||'-')+'</div>'+(s.error?'<div class="bad">'+esc(s.error)+'</div>':'')+'</div>').join('');services.innerHTML=list.map(s=>'<tr><td>'+esc(s.name)+'</td><td class="'+esc(s.status||'')+'">'+esc(s.status||'unknown')+'</td><td>'+esc(s.details||'')+'</td><td class="bad">'+esc(s.error||'')+'</td></tr>').join('')}

function openLogsModal(){logsModal.style.display='flex';loadLogs(false);if(logsPollInterval)clearInterval(logsPollInterval);logsPollInterval=setInterval(()=>loadLogs(false),2500)}
function closeLogsModal(){logsModal.style.display='none';if(logsPollInterval){clearInterval(logsPollInterval);logsPollInterval=null}}
async function loadLogs(notify=true){try{const r=await api('logs');rawLogsCache=r.logs||[];filterAndRenderLogs();if(notify)show('Logs refreshed')}catch(e){if(notify)show(e.message,true)}}
function filteredLogs(){const f=logFilter.value;if(f==='proxy')return rawLogsCache.filter(l=>l.module==='Proxy');if(f==='vpn')return rawLogsCache.filter(l=>l.module==='VPN');if(f==='system')return rawLogsCache.filter(l=>l.module!=='Proxy'&&l.module!=='VPN');return rawLogsCache}
function filterAndRenderLogs(){const logs=filteredLogs();logRows.innerHTML=logs.length?logs.slice(-200).reverse().map(l=>'<tr><td>'+esc(l.timestamp||l.time)+'</td><td>'+esc(l.level)+'</td><td>'+esc(l.module)+'</td><td>'+esc(l.message)+'</td></tr>').join(''):'<tr><td colspan="4" class="muted" style="text-align:center;padding:24px">No logs for this filter.</td></tr>'}
function logText(){return filteredLogs().map(l=>'['+(l.timestamp||l.time||'')+'] ['+(l.level||'')+'] ['+(l.module||'')+'] '+(l.message||'')).join('\n')}
function copyLogContent(){const text=logText();if(!text){alert('No log content to copy.');return}navigator.clipboard&&navigator.clipboard.writeText?navigator.clipboard.writeText(text).then(()=>show('Logs copied')).catch(()=>fallbackCopy(text)):fallbackCopy(text)}
function fallbackCopy(text){const ta=document.createElement('textarea');ta.value=text;document.body.appendChild(ta);ta.select();document.execCommand('copy');document.body.removeChild(ta);show('Logs copied')}
function exportLogContent(){const text=logText();if(!text){alert('No log content to export.');return}const blob=new Blob([text],{type:'text/plain;charset=utf-8'});const url=URL.createObjectURL(blob);const a=document.createElement('a');a.href=url;a.download='vpngate_log_'+logFilter.value+'_'+new Date().toISOString().slice(0,10)+'.txt';document.body.appendChild(a);a.click();document.body.removeChild(a);URL.revokeObjectURL(url)}

['click','keydown','mousemove','touchstart'].forEach(evt=>document.addEventListener(evt,()=>{lastInteraction=Date.now()},{passive:true}));
setInterval(async()=>{if(document.hidden||refreshInFlight)return;if(current.state&&current.state.is_connecting)return;if(Date.now()-lastInteraction<5000)return;await load(true)},10000);
load(false).catch(e=>show(e.message,true));
</script>
</body>
</html>`
