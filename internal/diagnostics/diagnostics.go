package diagnostics

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/6Kmfi6HP/aimili-vpngate/internal/config"
)

type Check struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func RuntimeChecks(cfg config.Config) []Check {
	checks := []Check{
		commandCheck("openvpn", cfg.OpenVPNCmd),
		commandCheck("ip", "ip"),
		commandCheck("iptables", "iptables"),
		tunCheck(),
		dataDirCheck(cfg.DataDir),
		portCheck("management-ui", cfg.UIHost, cfg.UIPort),
		portCheck("local-proxy", cfg.LocalProxyHost, cfg.LocalProxyPort),
	}
	return checks
}

func commandCheck(name, command string) Check {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return Check{Name: name, OK: false, Message: "empty command"}
	}
	if _, err := exec.LookPath(fields[0]); err != nil {
		return Check{Name: name, OK: false, Message: fmt.Sprintf("%s not found in PATH", fields[0])}
	}
	return Check{Name: name, OK: true, Message: fields[0] + " found"}
}

func tunCheck() Check {
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		return Check{Name: "tun", OK: false, Message: "/dev/net/tun is missing or inaccessible"}
	}
	return Check{Name: "tun", OK: true, Message: "/dev/net/tun is available"}
}

func dataDirCheck(dir string) Check {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Check{Name: "data-dir", OK: false, Message: err.Error()}
	}
	probe := dir + "/.write-test"
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return Check{Name: "data-dir", OK: false, Message: err.Error()}
	}
	_ = os.Remove(probe)
	return Check{Name: "data-dir", OK: true, Message: dir + " is writable"}
}

func portCheck(name, host string, port int) Check {
	if port <= 0 || port > 65535 {
		return Check{Name: name, OK: false, Message: "port is outside 1-65535"}
	}
	bindHost := host
	if bindHost == "" || bindHost == "::" {
		bindHost = "::1"
	}
	if bindHost == "0.0.0.0" {
		bindHost = "127.0.0.1"
	}
	addr := net.JoinHostPort(bindHost, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, 150*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		return Check{Name: name, OK: true, Message: addr + " is reachable"}
	}
	return Check{Name: name, OK: true, Message: addr + " is not currently listening"}
}
