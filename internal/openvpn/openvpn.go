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
	"strings"
	"sync"
	"time"

	"github.com/6Kmfi6HP/aimili-vpngate/internal/config"
	"github.com/6Kmfi6HP/aimili-vpngate/internal/vpngate"
)

type Logger interface {
	Printf(format string, args ...any)
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
	parts := splitCommand(m.cfg.OpenVPNCmd)
	if len(parts) == 0 {
		parts = []string{"openvpn"}
	}
	args := append([]string{}, parts[1:]...)
	args = append(args, "--config", configPath, "--auth-user-pass", m.auth)
	runCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(runCtx, parts[0], args...)
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
	go m.watchOutput(io.MultiReader(stdout, stderr), ready)
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
		return fmt.Errorf("openvpn timeout after %ds", m.cfg.OpenVPNTestTimeoutSeconds)
	case <-ctx.Done():
		m.Stop()
		return ctx.Err()
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

func (m *Manager) watchOutput(r io.Reader, ready chan<- error) {
	readySent := false
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		m.appendLog(line)
		if m.logger != nil {
			m.logger.Printf("[OpenVPN] %s", line)
		}
		if !readySent {
			switch {
			case strings.Contains(line, "Initialization Sequence Completed"):
				ready <- nil
				readySent = true
			case strings.Contains(line, "AUTH_FAILED"):
				ready <- errors.New("openvpn authentication failed")
				readySent = true
			case strings.Contains(line, "TLS Error"):
				ready <- errors.New("openvpn TLS error")
				readySent = true
			case strings.Contains(line, "OPTIONS ERROR"):
				ready <- errors.New(line)
				readySent = true
			case strings.Contains(line, "Failed to open tun/tap interface"):
				ready <- errors.New("openvpn failed to open tun/tap interface")
				readySent = true
			case strings.Contains(line, "process-push-msg-failed"):
				ready <- errors.New("openvpn failed to apply pushed options")
				readySent = true
			}
		}
	}
	if !readySent {
		if err := scanner.Err(); err != nil {
			ready <- err
		} else {
			ready <- errors.New("openvpn exited before readiness")
		}
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
