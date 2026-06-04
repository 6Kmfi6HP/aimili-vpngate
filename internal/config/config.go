package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultAPIURL = "https://www.vpngate.net/api/iphone/"

type Config struct {
	Version                   string
	APIURL                    string
	FetchIntervalSeconds      int
	CheckIntervalSeconds      int
	TargetValidNodes          int
	MaxScanRows               int
	OpenVPNTestTimeoutSeconds int
	OpenVPNCmd                string
	OpenVPNAuthUser           string
	OpenVPNAuthPass           string
	LocalProxyHost            string
	LocalProxyPort            int
	LocalProxyOutboundDevice  string
	UIHost                    string
	UIPort                    int
	InvalidBackoffSeconds     int
	DataDir                   string
	ContainerMode             bool
	AutoConnect               bool
	IPEnrichment              bool
	ProxyCheckURLs            []string
}

func Load(version string) Config {
	cfg := Config{
		Version:                   version,
		APIURL:                    getenv("VPNGATE_API_URL", DefaultAPIURL),
		FetchIntervalSeconds:      getenvInt("FETCH_INTERVAL_SECONDS", 960),
		CheckIntervalSeconds:      getenvInt("CHECK_INTERVAL_SECONDS", 960),
		TargetValidNodes:          getenvInt("TARGET_VALID_NODES", 3),
		MaxScanRows:               getenvInt("MAX_SCAN_ROWS", 300),
		OpenVPNTestTimeoutSeconds: getenvInt("OPENVPN_TEST_TIMEOUT_SECONDS", 35),
		OpenVPNCmd:                getenv("OPENVPN_CMD", "openvpn"),
		OpenVPNAuthUser:           getenv("OPENVPN_AUTH_USER", "vpn"),
		OpenVPNAuthPass:           getenv("OPENVPN_AUTH_PASS", "vpn"),
		LocalProxyHost:            getenv("LOCAL_PROXY_HOST", "127.0.0.1"),
		LocalProxyPort:            getenvInt("LOCAL_PROXY_PORT", 7928),
		LocalProxyOutboundDevice:  outboundDevice(),
		UIHost:                    getenv("UI_HOST", "::"),
		UIPort:                    getenvInt("UI_PORT", 8787),
		InvalidBackoffSeconds:     getenvInt("INVALID_BACKOFF_SECONDS", 30*60),
		ContainerMode:             getenvBool("AIMILIVPN_CONTAINER", false),
		AutoConnect:               getenvBool("AIMILIVPN_AUTOCONNECT", true),
		IPEnrichment:              getenvBool("AIMILIVPN_IP_ENRICHMENT", true),
		ProxyCheckURLs:            getenvCSV("AIMILIVPN_PROXY_CHECK_URLS", []string{"http://ip.sb", "http://api.ipify.org"}),
	}
	cfg.DataDir = dataDir(cfg.ContainerMode)
	return cfg
}

func outboundDevice() string {
	value := os.Getenv("LOCAL_PROXY_OUTBOUND_DEVICE")
	if value == "" {
		return "tun0"
	}
	if value == "none" || value == "off" || value == "-" {
		return ""
	}
	return value
}

func getenv(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func getenvInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvCSV(name string, fallback []string) []string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func dataDir(container bool) string {
	if value := os.Getenv("VPNGATE_DATA_DIR"); value != "" {
		if abs, err := filepath.Abs(value); err == nil {
			return abs
		}
		return value
	}
	if container {
		return "/data"
	}
	exe, err := os.Executable()
	if err != nil {
		return "vpngate_data"
	}
	return filepath.Join(filepath.Dir(exe), "vpngate_data")
}
