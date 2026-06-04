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
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "services": services})
	case path == "/api/logs":
		logs, err := s.Backend.Logs()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
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

const indexHTML = `<!doctype html>
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
