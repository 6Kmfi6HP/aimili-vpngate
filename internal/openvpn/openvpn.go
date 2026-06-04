package openvpn

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/6Kmfi6HP/aimili-vpngate/internal/config"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/vpngate"
)

type Logger interface {
	Printf(format string, args ...any)
}

type CommandOptions struct {
	ConfigPath  string
	Device      string
	RouteNoPull bool
	ConfigText  string
}

type ProbeResult struct {
	OK      bool
	Message string
}

type Manager struct {
	cfg     config.Config
	auth    string
	logger  Logger
	mu      sync.Mutex
	cmd     *exec.Cmd
	nodeID  string
	cancel  context.CancelFunc
	logTail []string
}

func NewManager(cfg config.Config, authFile string, logger Logger) *Manager {
	return &Manager{cfg: cfg, auth: authFile, logger: logger}
}

func (m *Manager) EnsureAuthFile() error {
	if err := os.MkdirAll(filepath.Dir(m.auth), 0o755); err != nil {
		return err
	}
	content := []byte(m.cfg.OpenVPNAuthUser + "\n" + m.cfg.OpenVPNAuthPass + "\n")
	return os.WriteFile(m.auth, content, 0o600)
}

func WriteConfig(configDir string, node vpngate.Node) (string, error) {
	if node.ConfigText == "" {
		return "", errors.New("node has no OpenVPN config")
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(configDir, node.ID+".ovpn")
	return path, os.WriteFile(path, []byte(NormalizeConfig(node.ConfigText)), 0o600)
}

func BuildCommand(cfg config.Config, authFile string, opts CommandOptions) (string, []string) {
	parts := splitCommand(cfg.OpenVPNCmd)
	if len(parts) == 0 {
		parts = []string{"openvpn"}
	}
	device := opts.Device
	if device == "" {
		device = "tun0"
	}
	args := append([]string{}, parts[1:]...)
	args = append(args,
		"--config", opts.ConfigPath,
		"--dev", device,
		"--dev-type", "tun",
		"--pull-filter", "ignore", "route-ipv6",
		"--pull-filter", "ignore", "ifconfig-ipv6",
		"--route-delay", "2",
		"--connect-retry-max", "1",
		"--connect-timeout", "15",
		"--auth-user-pass", authFile,
		"--auth-nocache",
		"--data-ciphers", "AES-128-CBC:AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305",
		"--verb", "3",
	)
	if strings.Contains(strings.ToLower(opts.ConfigText), "proto tcp") || remoteLineUsesTCP(opts.ConfigText) {
		if kind, host, port := upstreamProxy(); host != "" && port != "" {
			if kind == "socks" {
				args = append(args, "--socks-proxy", host, port)
			} else {
				args = append(args, "--http-proxy", host, port)
			}
		}
	}
	if opts.RouteNoPull {
		args = append(args, "--route-nopull")
	}
	return parts[0], args
}

func NormalizeConfig(configText string) string {
	text := strings.ReplaceAll(configText, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	hasDataCiphers := false
	hasFallback := false
	hasDev := false
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") || strings.HasPrefix(fields[0], ";") {
			continue
		}
		switch fields[0] {
		case "dev":
			hasDev = true
			if len(fields) == 2 && fields[1] == "tun" {
				line = "dev tun0"
			}
		case "data-ciphers":
			hasDataCiphers = true
		case "data-ciphers-fallback":
			hasFallback = true
		}
		normalized = append(normalized, line)
	}
	text = strings.Join(normalized, "\n")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if !hasDev {
		text += "dev tun0\n"
	}
	if !hasDataCiphers {
		text += "data-ciphers AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305:AES-128-CBC\n"
	}
	if !hasFallback {
		text += "data-ciphers-fallback AES-128-CBC\n"
	}
	return text
}

func (m *Manager) Start(ctx context.Context, node vpngate.Node, configPath string) error {
	m.Stop()
	if err := m.EnsureAuthFile(); err != nil {
		return err
	}
	exe, args := BuildCommand(m.cfg, m.auth, CommandOptions{
		ConfigPath:  configPath,
		Device:      "tun0",
		RouteNoPull: true,
		ConfigText:  node.ConfigText,
	})
	runCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(runCtx, exe, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}
	m.mu.Lock()
	m.cmd = cmd
	m.nodeID = node.ID
	m.cancel = cancel
	m.mu.Unlock()
	ready := make(chan error, 1)
	go m.watchStreams(stdout, stderr, ready)
	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		if m.cmd == cmd {
			m.cmd = nil
			m.nodeID = ""
			m.cancel = nil
		}
		m.mu.Unlock()
		if err != nil && m.logger != nil {
			m.logger.Printf("openvpn exited: %v", err)
		}
	}()
	select {
	case err := <-ready:
		if err != nil {
			m.Stop()
		}
		return err
	case <-time.After(time.Duration(m.cfg.OpenVPNTestTimeoutSeconds) * time.Second):
		m.Stop()
		return fmt.Errorf("[ERR_OVPN_TIMEOUT] openvpn timeout after %ds", m.cfg.OpenVPNTestTimeoutSeconds)
	case <-ctx.Done():
		m.Stop()
		return ctx.Err()
	}
}

func (m *Manager) Probe(ctx context.Context, node vpngate.Node, configPath, device string, timeout time.Duration) ProbeResult {
	if timeout <= 0 {
		timeout = time.Duration(m.cfg.OpenVPNTestTimeoutSeconds) * time.Second
	}
	if device == "" {
		device = "tun2"
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	exe, args := BuildCommand(m.cfg, m.auth, CommandOptions{
		ConfigPath:  configPath,
		Device:      device,
		RouteNoPull: true,
		ConfigText:  node.ConfigText,
	})
	cmd := exec.CommandContext(runCtx, exe, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ProbeResult{Message: err.Error()}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return ProbeResult{Message: err.Error()}
	}
	if err := cmd.Start(); err != nil {
		return ProbeResult{Message: diagnoseStartError(err)}
	}
	ready := make(chan error, 1)
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()
	go m.watchStreams(stdout, stderr, ready)
	defer terminateAndWait(cmd, cancel, waitCh)
	select {
	case err := <-ready:
		if err != nil {
			return ProbeResult{Message: err.Error()}
		}
		return ProbeResult{OK: true, Message: "OpenVPN readiness probe succeeded"}
	case <-runCtx.Done():
		return ProbeResult{Message: fmt.Sprintf("[ERR_OVPN_TIMEOUT] openvpn probe timeout after %s", timeout)}
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	cmd := m.cmd
	cancel := m.cancel
	m.cmd = nil
	m.nodeID = ""
	m.cancel = nil
	m.mu.Unlock()
	terminateProcess(cmd, cancel)
}

func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && m.cmd.Process != nil
}

func (m *Manager) NodeID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodeID
}

func (m *Manager) LogTail() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.logTail))
	copy(out, m.logTail)
	return out
}

func terminateProcess(cmd *exec.Cmd, cancel context.CancelFunc) {
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
		time.AfterFunc(2*time.Second, func() {
			if cmd.ProcessState == nil {
				_ = cmd.Process.Kill()
			}
		})
	}
}

func terminateAndWait(cmd *exec.Cmd, cancel context.CancelFunc, waitCh <-chan error) {
	terminateProcess(cmd, cancel)
	if cmd == nil || waitCh == nil {
		return
	}
	select {
	case <-waitCh:
	case <-time.After(3 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-waitCh:
		case <-time.After(time.Second):
		}
	}
}

func (m *Manager) watchStreams(stdout, stderr io.Reader, ready chan<- error) {
	lines := make(chan string)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	scan := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		errs <- scanner.Err()
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	go func() {
		wg.Wait()
		close(lines)
		close(errs)
	}()
	readySent := false
	for line := range lines {
		m.appendLog(line)
		if m.logger != nil {
			m.logger.Printf("[OpenVPN] %s", line)
		}
		if !readySent {
			if handled, err := diagnoseOpenVPNLine(line); handled {
				ready <- err
				readySent = true
			}
		}
	}
	if !readySent {
		for err := range errs {
			if err != nil {
				ready <- err
				return
			}
		}
		ready <- errors.New("openvpn exited before readiness")
	}
}

func diagnoseOpenVPNLine(line string) (bool, error) {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(line, "Initialization Sequence Completed"):
		return true, nil
	case strings.Contains(lower, "net_iface_up: set ") && strings.HasSuffix(lower, " up"):
		return true, nil
	case strings.Contains(line, "AUTH_FAILED"):
		return true, errors.New("[ERR_OVPN_AUTH_FAILED] OpenVPN authentication failed")
	case strings.Contains(line, "TLS Error") || strings.Contains(lower, "tls key negotiation failed"):
		return true, errors.New("[ERR_OVPN_TLS_BLOCKED] OpenVPN TLS error")
	case strings.Contains(line, "OPTIONS ERROR"):
		return true, fmt.Errorf("[ERR_OVPN_OPTIONS] %s", line)
	case strings.Contains(line, "process-push-msg-failed"):
		return true, errors.New("[ERR_OVPN_PUSH_OPTIONS] OpenVPN failed to apply pushed options")
	case strings.Contains(lower, "cannot resolve host address"):
		return true, errors.New("[ERR_OVPN_DNS_RESOLVE] OpenVPN could not resolve node host")
	case strings.Contains(lower, "failed to open tun/tap") ||
		strings.Contains(lower, "cannot ioctl tunsetiff") ||
		strings.Contains(lower, "cannot open tun") ||
		(strings.Contains(lower, "operation not permitted") && (strings.Contains(lower, "tun") || strings.Contains(lower, "tap") || strings.Contains(lower, "net_admin"))):
		return true, errors.New("[ERR_OVPN_TUN_NOT_AVAILABLE] OpenVPN failed to open tun/tap interface")
	default:
		return false, nil
	}
}

func (m *Manager) appendLog(line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logTail = append(m.logTail, line)
	if len(m.logTail) > 80 {
		m.logTail = m.logTail[len(m.logTail)-80:]
	}
}

func splitCommand(command string) []string {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	return regexp.MustCompile(`\s+`).Split(command, -1)
}

func remoteLineUsesTCP(configText string) bool {
	for _, line := range strings.Split(configText, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 4 && strings.EqualFold(fields[0], "remote") && strings.Contains(strings.ToLower(fields[3]), "tcp") {
			return true
		}
	}
	return false
}

func upstreamProxy() (kind, host, port string) {
	for _, item := range []struct {
		name string
		kind string
	}{
		{"OPENVPN_UPSTREAM_SOCKS", "socks"},
		{"OPENVPN_UPSTREAM_HTTP", "http"},
		{"https_proxy", "http"},
		{"HTTPS_PROXY", "http"},
		{"http_proxy", "http"},
		{"HTTP_PROXY", "http"},
	} {
		value := os.Getenv(item.name)
		if value == "" {
			continue
		}
		kind, host, port = parseProxyValue(item.kind, value)
		if host != "" && port != "" {
			return kind, host, port
		}
	}
	return "", "", ""
}

func parseProxyValue(defaultKind, value string) (kind, host, port string) {
	kind = defaultKind
	value = strings.TrimSpace(value)
	if strings.Contains(value, "://") {
		parts := strings.SplitN(value, "://", 2)
		if strings.HasPrefix(parts[0], "socks") {
			kind = "socks"
		}
		value = parts[1]
	}
	value = strings.TrimSuffix(value, "/")
	if at := strings.LastIndex(value, "@"); at >= 0 {
		value = value[at+1:]
	}
	if h, p, ok := strings.Cut(value, ":"); ok {
		return kind, h, p
	}
	if defaultKind == "socks" {
		return kind, value, strconv.Itoa(10808)
	}
	return kind, value, strconv.Itoa(10808)
}

func diagnoseStartError(err error) string {
	if errors.Is(err, exec.ErrNotFound) {
		return "[ERR_OVPN_CMD_NOT_FOUND] openvpn command not found"
	}
	return "[ERR_OVPN_START_FAILED] " + err.Error()
}
