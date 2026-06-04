package vpngate

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Node struct {
	ID           string `json:"id"`
	Country      string `json:"country"`
	CountryShort string `json:"country_short"`
	CountryLong  string `json:"country_long"`
	IP           string `json:"ip"`
	HostName     string `json:"host_name"`
	Hostname     string `json:"hostname"`
	Ping         int    `json:"ping"`
	Speed        int64  `json:"speed"`
	Score        int    `json:"score"`
	Sessions     int    `json:"sessions"`
	Owner        string `json:"owner"`
	ASN          string `json:"asn"`
	ASName       string `json:"as_name"`
	Location     string `json:"location"`
	IPType       string `json:"ip_type"`
	Quality      string `json:"quality"`
	LatencyMS    int    `json:"latency_ms"`
	ConfigText   string `json:"config_text,omitempty"`
	ConfigFile   string `json:"config_file,omitempty"`
	Proto        string `json:"proto"`
	RemoteHost   string `json:"remote_host"`
	RemotePort   int    `json:"remote_port"`
	RemoteProto  string `json:"remote_proto"`
	ProbeStatus  string `json:"probe_status"`
	ProbeMessage string `json:"probe_message"`
	FetchedAt    int64  `json:"fetched_at"`
	ProbedAt     int64  `json:"probed_at"`
	InvalidUntil int64  `json:"invalid_until,omitempty"`
	Active       bool   `json:"active,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

type Client struct {
	APIURL string
	HTTP   *http.Client
}

func (c Client) Fetch(ctx context.Context) ([]Node, error) {
	attempts := c.fetchAttempts()
	var messages []string
	for _, attempt := range attempts {
		text, err := c.fetchText(ctx, attempt)
		if err == nil {
			return ParseAPI(text)
		}
		messages = append(messages, attempt.label+": "+err.Error())
	}
	return nil, fmt.Errorf("%s: %s", diagnoseFetch(messages), strings.Join(messages, " | "))
}

type fetchAttempt struct {
	label      string
	url        string
	insecure   bool
	proxyURL   *url.URL
	baseClient *http.Client
}

func (c Client) fetchText(ctx context.Context, attempt fetchAttempt) (string, error) {
	client := attempt.baseClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if attempt.proxyURL != nil || attempt.insecure {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		if attempt.proxyURL != nil {
			transport.Proxy = http.ProxyURL(attempt.proxyURL)
		}
		if attempt.insecure {
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		}
		client = &http.Client{Timeout: client.Timeout, Transport: transport}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, attempt.url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "AimiliVPN-Go/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vpngate api status %s", resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (c Client) fetchAttempts() []fetchAttempt {
	apiURL := c.APIURL
	if apiURL == "" {
		apiURL = "https://www.vpngate.net/api/iphone/"
	}
	var attempts []fetchAttempt
	if proxyURL := upstreamProxyURL(); proxyURL != nil {
		attempts = append(attempts, fetchAttempt{label: "upstream proxy", url: apiURL, proxyURL: proxyURL, baseClient: c.HTTP})
	}
	attempts = append(attempts, fetchAttempt{label: "https", url: apiURL, baseClient: c.HTTP})
	if strings.HasPrefix(apiURL, "https://") {
		attempts = append(attempts, fetchAttempt{label: "https insecure", url: apiURL, insecure: true, baseClient: c.HTTP})
		attempts = append(attempts, fetchAttempt{label: "http fallback", url: "http://" + strings.TrimPrefix(apiURL, "https://"), baseClient: c.HTTP})
	}
	return attempts
}

func upstreamProxyURL() *url.URL {
	for _, name := range []string{"OPENVPN_UPSTREAM_SOCKS", "OPENVPN_UPSTREAM_HTTP", "https_proxy", "HTTPS_PROXY", "http_proxy", "HTTP_PROXY"} {
		value := strings.TrimSpace(getenv(name))
		if value == "" {
			continue
		}
		if !strings.Contains(value, "://") {
			if strings.Contains(strings.ToLower(name), "socks") {
				value = "socks5://" + value
			} else {
				value = "http://" + value
			}
		}
		parsed, err := url.Parse(value)
		if err == nil && parsed.Host != "" {
			if parsed.Scheme == "socks" {
				parsed.Scheme = "socks5"
			}
			return parsed
		}
	}
	return nil
}

func getenv(name string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(os.Getenv(name)), "\n", ""))
}

func diagnoseFetch(messages []string) string {
	joined := strings.ToLower(strings.Join(messages, " "))
	switch {
	case strings.Contains(joined, "no such host"):
		return "[ERR_LOCAL_DNS_BROKEN]"
	case strings.Contains(joined, "certificate") || strings.Contains(joined, "tls"):
		return "[ERR_API_TLS_INTERFERENCE]"
	case strings.Contains(joined, "timeout") || strings.Contains(joined, "i/o timeout"):
		return "[ERR_API_IP_BLOCKED_OR_DOWN]"
	case strings.Contains(joined, "connection refused"):
		return "[ERR_API_IP_BLOCKED_OR_DOWN]"
	default:
		return "[ERR_API_FETCH_FAILED]"
	}
}

func ParseAPI(text string) ([]Node, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var csvLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		csvLines = append(csvLines, line)
	}
	if len(csvLines) == 0 {
		return nil, fmt.Errorf("empty vpngate response")
	}
	reader := csv.NewReader(strings.NewReader(strings.Join(csvLines, "\n")))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("missing vpngate rows")
	}
	header := map[string]int{}
	for i, name := range records[0] {
		header[name] = i
	}
	var nodes []Node
	for _, record := range records[1:] {
		cfg64 := field(record, header, "OpenVPN_ConfigData_Base64")
		if cfg64 == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(cfg64)
		if err != nil {
			continue
		}
		configText := string(decoded)
		host, port, proto := ParseRemote(configText, field(record, header, "IP"))
		ping := atoi(field(record, header, "Ping"))
		node := Node{
			Country:      field(record, header, "CountryShort"),
			CountryShort: field(record, header, "CountryShort"),
			CountryLong:  field(record, header, "CountryLong"),
			IP:           field(record, header, "IP"),
			HostName:     field(record, header, "HostName"),
			Hostname:     field(record, header, "HostName"),
			Ping:         ping,
			Speed:        int64(atoi(field(record, header, "Speed"))),
			Score:        atoi(field(record, header, "Score")),
			Sessions:     atoi(field(record, header, "NumVpnSessions")),
			LatencyMS:    ping,
			ConfigText:   configText,
			Proto:        proto,
			RemoteHost:   host,
			RemotePort:   port,
			RemoteProto:  proto,
			FetchedAt:    time.Now().Unix(),
			ProbeStatus:  "not_checked",
		}
		node.ID = StableID(node)
		nodes = append(nodes, node)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Ping == nodes[j].Ping {
			return nodes[i].Score > nodes[j].Score
		}
		if nodes[i].Ping <= 0 {
			return false
		}
		if nodes[j].Ping <= 0 {
			return true
		}
		return nodes[i].Ping < nodes[j].Ping
	})
	return nodes, nil
}

type ProbeFunc func(context.Context, Node) (bool, string)

func SelectCandidates(nodes []Node, maxRows int) []Node {
	if maxRows <= 0 || maxRows > len(nodes) {
		maxRows = len(nodes)
	}
	out := make([]Node, 0, maxRows)
	for _, node := range nodes[:maxRows] {
		if node.RemoteHost == "" || node.RemotePort == 0 {
			continue
		}
		out = append(out, node)
	}
	return out
}

func CheckCandidates(ctx context.Context, nodes []Node, targetValid int, timeout time.Duration) []Node {
	return CheckCandidatesWithProbe(ctx, nodes, targetValid, timeout, nil)
}

func CheckCandidatesWithProbe(ctx context.Context, nodes []Node, targetValid int, timeout time.Duration, probe ProbeFunc) []Node {
	if targetValid <= 0 {
		targetValid = len(nodes)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	skipTCPPrefilter := probe != nil && upstreamProxyURL() != nil
	type result struct {
		node Node
		ok   bool
	}
	jobs := make(chan Node)
	results := make(chan result)
	workers := 16
	if len(nodes) < workers {
		workers = len(nodes)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for node := range jobs {
				ok := false
				message := "TCP probe failed"
				if probe != nil && (skipTCPPrefilter || strings.Contains(strings.ToLower(node.RemoteProto), "udp")) {
					ok, message = probe(runCtx, node)
				} else {
					tcpOK := dialCheck(runCtx, node.RemoteHost, node.RemotePort, timeout)
					if tcpOK {
						ok = true
						message = "TCP probe succeeded"
					}
					if probe != nil {
						if tcpOK || strings.Contains(strings.ToLower(node.RemoteProto), "udp") {
							ok, message = probe(runCtx, node)
						} else {
							ok = false
							message = "TCP prefilter failed"
						}
					}
				}
				if ok {
					node.ProbeStatus = "available"
					node.ProbeMessage = message
				} else {
					node.ProbeStatus = "unavailable"
					node.ProbeMessage = message
				}
				node.ProbedAt = time.Now().Unix()
				results <- result{node: node, ok: ok}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, node := range nodes {
			select {
			case <-runCtx.Done():
				return
			case jobs <- node:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	var checked []Node
	valid := 0
	for res := range results {
		checked = append(checked, res.node)
		if res.ok {
			valid++
		}
		if valid >= targetValid {
			cancel()
		}
	}
	sort.SliceStable(checked, func(i, j int) bool {
		if checked[i].ProbeStatus == checked[j].ProbeStatus {
			return checked[i].Ping < checked[j].Ping
		}
		return isAvailable(checked[i].ProbeStatus)
	})
	return checked
}

func isAvailable(status string) bool {
	return status == "ok" || status == "available"
}

func ParseRemote(configText, fallbackIP string) (string, int, string) {
	proto := "udp"
	for _, line := range strings.Split(configText, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "proto":
			if len(fields) > 1 {
				proto = strings.ToLower(fields[1])
			}
		case "remote":
			host := fallbackIP
			port := 0
			if len(fields) > 1 {
				host = fields[1]
			}
			if len(fields) > 2 {
				port = atoi(fields[2])
			}
			if len(fields) > 3 {
				proto = strings.ToLower(fields[3])
			}
			return host, port, proto
		}
	}
	return fallbackIP, 0, proto
}

func StableID(node Node) string {
	base := node.IP
	if base == "" {
		base = node.RemoteHost
	}
	if base == "" {
		base = node.HostName
	}
	base = strings.ToLower(base)
	base = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "node"
	}
	if node.Country != "" {
		return strings.ToLower(node.Country) + "-" + base
	}
	return base
}

func field(record []string, header map[string]int, name string) string {
	i, ok := header[name]
	if !ok || i < 0 || i >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[i])
}

func atoi(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func dialCheck(ctx context.Context, host string, port int, timeout time.Duration) bool {
	if host == "" || port == 0 {
		return false
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
