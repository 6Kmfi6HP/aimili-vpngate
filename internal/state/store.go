package state

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	dir string
	mu  sync.Mutex
}

type UIConfig struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	ProxyPort    int    `json:"proxy_port"`
	SecretPath   string `json:"secret_path"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	RoutingMode  string `json:"routing_mode"`
	ForceCountry string `json:"force_country"`
	FixedNodeID  string `json:"fixed_node_id,omitempty"`
}

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Time      string `json:"time,omitempty"`
	Level     string `json:"level"`
	Module    string `json:"module"`
	Message   string `json:"message"`
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) ConfigDir() string {
	return filepath.Join(s.dir, "configs")
}

func (s *Store) AuthFile() string {
	return filepath.Join(s.dir, "vpngate_auth.txt")
}

func (s *Store) NodesFile() string {
	return filepath.Join(s.dir, "nodes.json")
}

func (s *Store) StateFile() string {
	return filepath.Join(s.dir, "state.json")
}

func (s *Store) UIConfigFile() string {
	return filepath.Join(s.dir, "ui_auth.json")
}

func (s *Store) LogFile() string {
	return filepath.Join(s.dir, "vpngate.log")
}

func (s *Store) Ensure() error {
	for _, dir := range []string{s.dir, s.ConfigDir(), filepath.Join(s.dir, "logs")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) EnsureWritable() error {
	if err := s.Ensure(); err != nil {
		return err
	}
	probe := filepath.Join(s.dir, ".write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return err
	}
	return os.Remove(probe)
}

func (s *Store) EnsureMigrationBackup() error {
	if err := s.Ensure(); err != nil {
		return err
	}
	marker := filepath.Join(s.dir, ".go-migration-backup-complete")
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	stamp := time.Now().Format("20060102-150405")
	backupDir := filepath.Join(s.dir, "backups", "go-migration-"+stamp)
	for _, path := range []string{s.UIConfigFile(), s.StateFile(), s.NodesFile(), s.AuthFile()} {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := os.MkdirAll(backupDir, 0o700); err != nil {
			return err
		}
		if err := copyFile(path, filepath.Join(backupDir, filepath.Base(path))); err != nil {
			return err
		}
	}
	return os.WriteFile(marker, []byte(backupDir+"\n"), 0o600)
}

func (s *Store) LoadUIConfig(defaultHost string, defaultPort, defaultProxyPort int) (UIConfig, error) {
	cfg := UIConfig{
		Host:        defaultHost,
		Port:        defaultPort,
		ProxyPort:   defaultProxyPort,
		SecretPath:  "EJsW2EeBo9lY",
		Username:    "admin",
		Password:    "",
		RoutingMode: "auto",
	}
	if err := ReadJSON(s.UIConfigFile(), &cfg); err != nil {
		if !os.IsNotExist(err) {
			return cfg, err
		}
		secret, err := randomString(12)
		if err == nil {
			cfg.SecretPath = secret
		}
		if err := s.SaveUIConfig(cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func (s *Store) SaveUIConfig(cfg UIConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := map[string]any{}
	raw, err := os.ReadFile(s.UIConfigFile())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}
	}
	data["host"] = cfg.Host
	data["port"] = cfg.Port
	data["proxy_port"] = cfg.ProxyPort
	data["secret_path"] = cfg.SecretPath
	data["username"] = cfg.Username
	data["password"] = cfg.Password
	data["routing_mode"] = cfg.RoutingMode
	data["force_country"] = cfg.ForceCountry
	if cfg.FixedNodeID != "" {
		data["fixed_node_id"] = cfg.FixedNodeID
	} else {
		delete(data, "fixed_node_id")
	}
	return WriteJSON(s.UIConfigFile(), data, 0o600)
}

func (s *Store) ReadState() (map[string]any, error) {
	var out map[string]any
	if err := ReadJSON(s.StateFile(), &out); err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func (s *Store) UpdateState(updates map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := map[string]any{}
	_ = ReadJSON(s.StateFile(), &state)
	for k, v := range updates {
		state[k] = v
	}
	return WriteJSON(s.StateFile(), state, 0o644)
}

func (s *Store) AppendLog(level, module, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := LogEntry{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Time:      time.Now().Format(time.RFC3339),
		Level:     level,
		Module:    module,
		Message:   message,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(s.dir, "logs"), 0o755); err != nil {
		return err
	}
	dayFile := filepath.Join(s.dir, "logs", time.Now().Format("2006-01-02")+".json")
	f, err := os.OpenFile(dayFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(raw, '\n'))
	return err
}

func (s *Store) ReadLogs() ([]LogEntry, error) {
	dayFile := filepath.Join(s.dir, "logs", time.Now().Format("2006-01-02")+".json")
	f, err := os.Open(dayFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var entries []LogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			if entry.Timestamp == "" {
				entry.Timestamp = entry.Time
			}
			if entry.Time == "" {
				entry.Time = entry.Timestamp
			}
			entries = append(entries, entry)
		}
	}
	return entries, scanner.Err()
}

func ReadJSON(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func WriteJSON(path string, value any, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if mode == 0 {
		mode = 0o644
	}
	return os.WriteFile(path, raw, mode)
}

func randomString(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", fmt.Errorf("random suffix: %w", err)
		}
		out[i] = alphabet[n.Int64()]
	}
	return string(out), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
