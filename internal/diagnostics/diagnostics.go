package diagnostics

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
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
		ipForwardCheck(),
		firewallPolicyCheck(cfg.LocalProxyPort),
		rpFilterCheck(),
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
		return Check{Name: "tun", OK: false, Message: Format(ErrOpenVPNTunUnavailable, TagOpenVPNTunUnavailable, "/dev/net/tun is missing or inaccessible")}
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

func ipForwardCheck() Check {
	if runtime.GOOS != "linux" {
		return Check{Name: "ip-forward", OK: true, Message: "not applicable on this OS"}
	}
	raw, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	if err != nil {
		return Check{Name: "ip-forward", OK: true, Message: "ip_forward not available"}
	}
	if strings.TrimSpace(string(raw)) == "0" {
		return Check{Name: "ip-forward", OK: false, Message: Format(ErrRouteForwardDisabled, TagRouteForwardDisabled, "/proc/sys/net/ipv4/ip_forward is 0")}
	}
	return Check{Name: "ip-forward", OK: true, Message: "IPv4 forwarding is enabled"}
}

func firewallPolicyCheck(proxyPort int) Check {
	if runtime.GOOS != "linux" {
		return Check{Name: "firewall-policy", OK: true, Message: "not applicable on this OS"}
	}
	if out, err := commandOutput("ufw", "status"); err == nil && strings.Contains(out, "Status: active") && !strings.Contains(out, strconv.Itoa(proxyPort)) {
		return Check{Name: "firewall-policy", OK: false, Message: Format(ErrFirewallBlockingForward, TagFirewallBlockingForward, fmt.Sprintf("UFW is active without an allow rule for proxy port %d", proxyPort))}
	}
	if out, err := commandOutput("systemctl", "is-active", "firewalld"); err == nil && strings.TrimSpace(out) == "active" {
		return Check{Name: "firewall-policy", OK: false, Message: Format(ErrFirewallBlockingForward, TagFirewallBlockingForward, "firewalld is active; ensure proxy port and tun0 forwarding are allowed")}
	}
	if out, err := commandOutput("iptables", "-S"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			switch strings.TrimSpace(line) {
			case "-P OUTPUT DROP":
				return Check{Name: "firewall-policy", OK: false, Message: Format(ErrFirewallBlockingForward, TagFirewallBlockingForward, "iptables OUTPUT default policy is DROP")}
			case "-P FORWARD DROP":
				return Check{Name: "firewall-policy", OK: false, Message: Format(ErrFirewallBlockingForward, TagFirewallBlockingForward, "iptables FORWARD default policy is DROP")}
			}
		}
	}
	return Check{Name: "firewall-policy", OK: true, Message: "no blocking firewall policy detected"}
}

func rpFilterCheck() Check {
	if runtime.GOOS != "linux" {
		return Check{Name: "rp-filter", OK: true, Message: "not applicable on this OS"}
	}
	for _, path := range []string{
		"/proc/sys/net/ipv4/conf/tun0/rp_filter",
		"/proc/sys/net/ipv4/conf/all/rp_filter",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(raw)) == "1" {
			return Check{Name: "rp-filter", OK: false, Message: Format(ErrRouteRPFilterStrict, TagRouteRPFilterStrict, path+" is strict (1)")}
		}
		return Check{Name: "rp-filter", OK: true, Message: path + " is not strict"}
	}
	return Check{Name: "rp-filter", OK: true, Message: "rp_filter not available"}
}

func commandOutput(name string, args ...string) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", err
	}
	cmd := exec.Command(name, args...)
	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
		return string(out), err
	case <-time.After(2 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return string(out), fmt.Errorf("%s timed out", name)
	}
}
