package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const (
	serviceName = "aimilivpn"
	installDir  = "/opt/aimilivpn"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "start", "stop", "restart":
		err = service(cmd)
	case "status":
		err = status()
	case "logs":
		err = logs(args)
	case "update":
		err = runInstaller("upgrade", args...)
	case "uninstall":
		err = runInstaller("uninstall", args...)
	case "web":
		err = setUI(args)
	case "port":
		err = setPorts(args)
	case "password":
		err = setPassword(args)
	case "help", "-h", "--help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("Usage: ml <start|stop|restart|status|logs|update|uninstall|web|port|password>")
}

func service(action string) error {
	if _, err := exec.LookPath("systemctl"); err == nil {
		return run("systemctl", action, serviceName+".service")
	}
	if _, err := exec.LookPath("rc-service"); err == nil {
		return run("rc-service", serviceName, action)
	}
	return fmt.Errorf("no supported service manager found")
}

func status() error {
	cfg, _ := loadUIConfig()
	state, _ := loadMap(filepath.Join(dataDir(), "state.json"))
	fmt.Printf("AimiliVPN service: %s\n", serviceState())
	fmt.Printf("Management UI: http://%s:%d/%s/\n", cfg.Host, cfg.Port, cfg.SecretPath)
	fmt.Printf("Local proxy: %s:%d\n", proxyHost(), cfg.ProxyPort)
	fmt.Printf("Data dir: %s\n", dataDir())
	if len(state) > 0 {
		fmt.Printf("Active node: %v\n", state["active_openvpn_node_id"])
		fmt.Printf("Last message: %v\n", state["last_check_message"])
	}
	return nil
}

func logs(args []string) error {
	if logFile := preferredLogFile(); logFile != "" {
		tailArgs := append([]string{"-n", "120"}, args...)
		tailArgs = append(tailArgs, logFile)
		if _, err := exec.LookPath("tail"); err == nil {
			return run("tail", tailArgs...)
		}
		raw, err := os.ReadFile(logFile)
		if err != nil {
			return err
		}
		fmt.Print(string(raw))
		return nil
	}
	if _, err := exec.LookPath("journalctl"); err == nil {
		journalArgs := append([]string{"-u", serviceName + ".service", "-n", "120", "--no-pager"}, args...)
		return run("journalctl", journalArgs...)
	}
	return fmt.Errorf("no AimiliVPN log file found under %s", dataDir())
}

func preferredLogFile() string {
	for _, path := range []string{
		filepath.Join(dataDir(), "logs", currentDay()+".json"),
		filepath.Join(dataDir(), "vpngate.log"),
	} {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && info.Size() > 0 {
			return path
		}
	}
	return ""
}

func setUI(args []string) error {
	cfg, err := loadUIConfig()
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] != "" {
		cfg.SecretPath = args[0]
	}
	return saveUIConfig(cfg)
}

func setPorts(args []string) error {
	cfg, err := loadUIConfig()
	if err != nil {
		return err
	}
	if len(args) > 0 {
		port, err := strconv.Atoi(args[0])
		if err != nil {
			return err
		}
		cfg.Port = port
	}
	if len(args) > 1 {
		port, err := strconv.Atoi(args[1])
		if err != nil {
			return err
		}
		cfg.ProxyPort = port
	}
	return saveUIConfig(cfg)
}

func setPassword(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("password command requires a password argument")
	}
	cfg, err := loadUIConfig()
	if err != nil {
		return err
	}
	cfg.Password = args[0]
	return saveUIConfig(cfg)
}

func runInstaller(action string, args ...string) error {
	installer := filepath.Join(installDir, "install.sh")
	if _, err := os.Stat(installer); err != nil {
		return fmt.Errorf("installer not found at %s", installer)
	}
	return run(installer, append([]string{action}, args...)...)
}

func serviceState() string {
	if _, err := exec.LookPath("systemctl"); err == nil {
		out, err := exec.Command("systemctl", "is-active", serviceName+".service").Output()
		if err == nil {
			return stringTrim(out)
		}
	}
	return "unknown"
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

type uiConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	ProxyPort  int    `json:"proxy_port"`
	SecretPath string `json:"secret_path"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Raw        map[string]any
}

func loadUIConfig() (uiConfig, error) {
	cfg := uiConfig{Host: "::", Port: 8787, ProxyPort: 7928, SecretPath: "EJsW2EeBo9lY", Username: "admin"}
	path := filepath.Join(dataDir(), "ui_auth.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	_ = json.Unmarshal(raw, &cfg.Raw)
	return cfg, nil
}

func saveUIConfig(cfg uiConfig) error {
	if err := os.MkdirAll(dataDir(), 0o755); err != nil {
		return err
	}
	data := cfg.Raw
	if data == nil {
		data = map[string]any{}
	}
	data["host"] = cfg.Host
	data["port"] = cfg.Port
	data["proxy_port"] = cfg.ProxyPort
	data["secret_path"] = cfg.SecretPath
	data["username"] = cfg.Username
	data["password"] = cfg.Password
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir(), "ui_auth.json"), append(raw, '\n'), 0o600)
}

func loadMap(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	return out, json.Unmarshal(raw, &out)
}

func dataDir() string {
	if value := os.Getenv("VPNGATE_DATA_DIR"); value != "" {
		return value
	}
	return filepath.Join(installDir, "vpngate_data")
}

func proxyHost() string {
	if value := os.Getenv("LOCAL_PROXY_HOST"); value != "" {
		return value
	}
	return "127.0.0.1"
}

func stringTrim(raw []byte) string {
	s := string(raw)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}

func currentDay() string {
	if value := os.Getenv("AIMILIVPN_LOG_DAY"); value != "" {
		return value
	}
	return time.Now().Format("2006-01-02")
}
