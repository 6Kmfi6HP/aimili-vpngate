package vpngate

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"net/http"
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
	CountryLong  string `json:"country_long,omitempty"`
	IP           string `json:"ip"`
	HostName     string `json:"hostname,omitempty"`
	Ping         int    `json:"ping"`
	Speed        int64  `json:"speed,omitempty"`
	Score        int    `json:"score,omitempty"`
	ConfigText   string `json:"config_text,omitempty"`
	ConfigFile   string `json:"config_file,omitempty"`
	RemoteHost   string `json:"remote_host,omitempty"`
	RemotePort   int    `json:"remote_port,omitempty"`
	RemoteProto  string `json:"remote_proto,omitempty"`
	ProbeStatus  string `json:"probe_status,omitempty"`
	ProbeMessage string `json:"probe_message,omitempty"`
	InvalidUntil int64  `json:"invalid_until,omitempty"`
	Active       bool   `json:"active,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

type Client struct {
	APIURL string
	HTTP   *http.Client
}

func (c Client) Fetch(ctx context.Context) ([]Node, error) {
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.APIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AimiliVPN-Go/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vpngate api status %s", resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	return ParseAPI(string(raw))
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
		node := Node{
			Country:     field(record, header, "CountryShort"),
			CountryLong: field(record, header, "CountryLong"),
			IP:          field(record, header, "IP"),
			HostName:    field(record, header, "HostName"),
			Ping:        atoi(field(record, header, "Ping")),
			Speed:       int64(atoi(field(record, header, "Speed"))),
			Score:       atoi(field(record, header, "Score")),
			ConfigText:  configText,
			RemoteHost:  host,
			RemotePort:  port,
			RemoteProto: proto,
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
	if targetValid <= 0 {
		targetValid = len(nodes)
	}
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
				ok := dialCheck(ctx, node.RemoteHost, node.RemotePort, timeout)
				if ok {
					node.ProbeStatus = "ok"
					node.ProbeMessage = "TCP probe succeeded"
				} else {
					node.ProbeStatus = "unavailable"
					node.ProbeMessage = "TCP probe failed"
				}
				results <- result{node: node, ok: ok}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, node := range nodes {
			select {
			case <-ctx.Done():
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
			// Keep draining goroutines through context cancellation at caller discretion.
		}
	}
	sort.SliceStable(checked, func(i, j int) bool {
		if checked[i].ProbeStatus == checked[j].ProbeStatus {
			return checked[i].Ping < checked[j].Ping
		}
		return checked[i].ProbeStatus == "ok"
	})
	return checked
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
