package web

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
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
		cfg := s.Backend.UIConfig()
		updateSettings(payload, &cfg)
		if err := s.Backend.UpdateUIConfig(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restart_needed": false, "message": "settings updated"})
	case "/api/update_routing":
		cfg := s.Backend.UIConfig()
		updateRouting(payload, &cfg)
		if err := s.Backend.UpdateUIConfig(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "routing updated"})
	case "/api/check":
		msg, err := s.Backend.RefreshNodes(r.Context(), true)
		writeResult(w, msg, err)
	case "/api/refresh_nodes":
		go func() {
			_, _ = s.Backend.RefreshNodes(context.Background(), false)
		}()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "node refresh started"})
	case "/api/test_nodes":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "nodes": []vpngate.Node{}})
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

func updateSettings(payload map[string]any, cfg *state.UIConfig) {
	if value, ok := payload["secret_path"].(string); ok && value != "" {
		cfg.SecretPath = value
	}
	if value, ok := payload["routing_mode"].(string); ok && value != "" {
		cfg.RoutingMode = value
	}
	if value, ok := payload["force_country"].(string); ok {
		cfg.ForceCountry = value
	}
	if value, ok := numberValue(payload["port"]); ok {
		cfg.Port = value
	}
	if value, ok := numberValue(payload["proxy_port"]); ok {
		cfg.ProxyPort = value
	}
}

func updateRouting(payload map[string]any, cfg *state.UIConfig) {
	if value, ok := payload["routing_mode"].(string); ok && value != "" {
		cfg.RoutingMode = value
	}
	if value, ok := payload["force_country"].(string); ok {
		cfg.ForceCountry = value
	}
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

const loginHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>AimiliVPN Login</title><style>body{font-family:sans-serif;margin:3rem;max-width:32rem}input,button{font:inherit;padding:.7rem;margin:.3rem 0;width:100%}</style></head>
<body><h1>AimiliVPN</h1><form id="f"><input name="username" placeholder="admin" autocomplete="username"><input name="password" type="password" placeholder="password" autocomplete="current-password"><button>Login</button></form><p id="m"></p><script>
f.onsubmit=async e=>{e.preventDefault();const d=Object.fromEntries(new FormData(f));const r=await fetch('./api/login',{method:'POST',body:JSON.stringify(d)});if(r.ok) location.reload(); else m.textContent='Login failed';};
</script></body></html>`

const indexHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>AimiliVPN</title><style>body{font-family:sans-serif;margin:2rem;background:#111;color:#eee}button{padding:.55rem .8rem;margin:.25rem}pre{background:#1b1b1b;padding:1rem;overflow:auto}</style></head>
<body><h1>AimiliVPN</h1><p>Go runtime management UI</p><button onclick="refresh()">Refresh nodes</button><button onclick="status()">Gateway status</button><button onclick="logs()">Logs</button><pre id="out">Loading...</pre><script>
async function load(){out.textContent=JSON.stringify(await (await fetch('./api/nodes')).json(),null,2)}
async function refresh(){out.textContent=JSON.stringify(await (await fetch('./api/refresh_nodes',{method:'POST'})).json(),null,2)}
async function status(){out.textContent=JSON.stringify(await (await fetch('./api/gateway_status')).json(),null,2)}
async function logs(){out.textContent=JSON.stringify(await (await fetch('./api/logs')).json(),null,2)}
load();
</script></body></html>`
