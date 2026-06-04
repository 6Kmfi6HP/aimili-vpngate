package vpngate

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseAPIFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "vpngate_api_sample.csv"))
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := ParseAPI(string(raw))
	if err != nil {
		t.Fatalf("ParseAPI: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len(nodes) = %d", len(nodes))
	}
	node := nodes[0]
	if node.ID != "jp-198-51-100-10" {
		t.Fatalf("ID = %q", node.ID)
	}
	if node.RemoteHost != "198.51.100.10" || node.RemotePort != 443 || node.RemoteProto != "tcp" {
		t.Fatalf("remote not parsed: %#v", node)
	}
	if node.ConfigText == "" {
		t.Fatal("ConfigText is empty")
	}
	if node.LatencyMS != node.Ping {
		t.Fatalf("LatencyMS = %d, want ping %d", node.LatencyMS, node.Ping)
	}
	nodeJSON, err := json.Marshal(Node{ID: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"owner":""`, `"probe_message":""`, `"latency_ms":0`, `"remote_port":0`} {
		if !strings.Contains(string(nodeJSON), want) {
			t.Fatalf("compatible field missing %s from %s", want, nodeJSON)
		}
	}
}

func TestSelectCandidatesSkipsMissingRemote(t *testing.T) {
	nodes := []Node{{ID: "a", RemoteHost: "127.0.0.1", RemotePort: 443}, {ID: "b"}}
	got := SelectCandidates(nodes, 10)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("unexpected candidates: %#v", got)
	}
}

func TestCheckCandidatesWithProbeDoesNotAcceptTCPOnlyAndKeepsUDP(t *testing.T) {
	clearProxyEnv(t)
	nodes := []Node{
		{ID: "tcp", RemoteHost: "127.0.0.1", RemotePort: 1, RemoteProto: "tcp", Ping: 10},
		{ID: "udp", RemoteHost: "203.0.113.1", RemotePort: 1194, RemoteProto: "udp", Ping: 20},
	}
	var probed []string
	checked := CheckCandidatesWithProbe(context.Background(), nodes, 2, time.Millisecond, func(_ context.Context, node Node) (bool, string) {
		probed = append(probed, node.ID)
		return node.ID == "udp", "openvpn probe"
	})
	if len(probed) != 1 || probed[0] != "udp" {
		t.Fatalf("expected only UDP direct probe after TCP prefilter, got %#v", probed)
	}
	for _, node := range checked {
		if node.ID == "tcp" && node.ProbeStatus == "available" {
			t.Fatalf("TCP-only result marked available: %#v", node)
		}
		if node.ID == "udp" && node.ProbeStatus != "available" {
			t.Fatalf("UDP probe was not accepted: %#v", node)
		}
	}
}

func TestCheckCandidatesStopsAfterTargetValid(t *testing.T) {
	clearProxyEnv(t)
	nodes := make([]Node, 80)
	for i := range nodes {
		nodes[i] = Node{ID: fmt.Sprintf("node-%d", i), RemoteHost: "203.0.113.1", RemotePort: 1194, RemoteProto: "udp", Ping: i + 1}
	}
	var probes atomic.Int32
	checked := CheckCandidatesWithProbe(context.Background(), nodes, 2, time.Millisecond, func(_ context.Context, node Node) (bool, string) {
		probes.Add(1)
		return true, "openvpn probe"
	})
	probeCount := int(probes.Load())
	if probeCount >= len(nodes) {
		t.Fatalf("targetValid did not stop probing: probes=%d nodes=%d checked=%d", probeCount, len(nodes), len(checked))
	}
	if probeCount > 20 {
		t.Fatalf("too many probes after target reached: %d", probeCount)
	}
}

func TestCheckCandidatesSkipsTCPPrefilterWhenUpstreamProxyConfigured(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("OPENVPN_UPSTREAM_SOCKS", "127.0.0.1:1080")
	nodes := []Node{{ID: "tcp", RemoteHost: "203.0.113.1", RemotePort: 443, RemoteProto: "tcp", Ping: 10}}
	probed := false
	checked := CheckCandidatesWithProbe(context.Background(), nodes, 1, time.Millisecond, func(_ context.Context, node Node) (bool, string) {
		probed = true
		return true, "openvpn probe through upstream proxy"
	})
	if !probed {
		t.Fatalf("TCP node was rejected by direct prefilter despite upstream proxy")
	}
	if len(checked) != 1 || checked[0].ProbeStatus != "available" {
		t.Fatalf("unexpected checked nodes: %#v", checked)
	}
}

func TestFetchFallsBackFromHTTPSToHTTP(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "vpngate_api_sample.csv"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	apiURL := "https://" + strings.TrimPrefix(server.URL, "http://")
	nodes, err := Client{APIURL: apiURL}.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes = %d", len(nodes))
	}
}

func TestFetchAttemptsIncludeUpstreamProxy(t *testing.T) {
	t.Setenv("OPENVPN_UPSTREAM_SOCKS", "")
	t.Setenv("OPENVPN_UPSTREAM_HTTP", "127.0.0.1:18080")
	attempts := Client{APIURL: "https://example.com/api"}.fetchAttempts()
	if len(attempts) == 0 || attempts[0].proxyURL == nil {
		t.Fatalf("proxy attempt missing: %#v", attempts)
	}
	if attempts[0].proxyURL.String() != "http://127.0.0.1:18080" {
		t.Fatalf("proxy URL = %s", attempts[0].proxyURL)
	}
}

func TestFetchAttemptsNormalizeSocksScheme(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("OPENVPN_UPSTREAM_SOCKS", "socks://127.0.0.1:1080")
	attempts := Client{APIURL: "https://example.com/api"}.fetchAttempts()
	if len(attempts) == 0 || attempts[0].proxyURL == nil {
		t.Fatalf("proxy attempt missing: %#v", attempts)
	}
	if attempts[0].proxyURL.String() != "socks5://127.0.0.1:1080" {
		t.Fatalf("proxy URL = %s", attempts[0].proxyURL)
	}
}

func TestFetchTextSupportsSOCKSUpstreamProxy(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "vpngate_api_sample.csv"))
	if err != nil {
		t.Fatal(err)
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Connection", "close")
		_, _ = w.Write(raw)
	}))
	defer target.Close()

	proxyURL, stop := startSOCKSRelay(t)
	defer stop()

	text, err := Client{}.fetchText(context.Background(), fetchAttempt{
		label:    "socks",
		url:      target.URL,
		proxyURL: proxyURL,
	})
	if err != nil {
		t.Fatalf("fetchText via SOCKS: %v", err)
	}
	if !strings.Contains(text, "OpenVPN_ConfigData_Base64") {
		t.Fatalf("unexpected response body: %q", text)
	}
}

func TestDiagnoseFetchUsesActiveProbes(t *testing.T) {
	cases := []struct {
		name   string
		lookup func(context.Context, string) ([]net.IPAddr, error)
		dial   func(context.Context, string) error
		want   string
	}{
		{
			name: "local DNS broken",
			lookup: func(context.Context, string) ([]net.IPAddr, error) {
				return nil, errors.New("dns down")
			},
			dial: func(context.Context, string) error { return errors.New("offline") },
			want: "[1006] ERR_LOCAL_DNS_BROKEN",
		},
		{
			name: "api domain blocked",
			lookup: func(_ context.Context, host string) ([]net.IPAddr, error) {
				if host == "api.example" {
					return nil, errors.New("api blocked")
				}
				return []net.IPAddr{{IP: net.ParseIP("192.0.2.1")}}, nil
			},
			dial: func(context.Context, string) error { return nil },
			want: "[1007] ERR_API_DOMAIN_BLOCKED",
		},
		{
			name: "api ip blocked",
			lookup: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
			},
			dial: func(_ context.Context, address string) error {
				if strings.HasPrefix(address, "8.8.8.8:") {
					return nil
				}
				return errors.New("blocked")
			},
			want: "[1008] ERR_API_IP_BLOCKED_OR_DOWN",
		},
		{
			name: "vps offline",
			lookup: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
			},
			dial: func(context.Context, string) error { return errors.New("offline") },
			want: "[1009] ERR_VPS_OUTBOUND_BLOCKED",
		},
		{
			name: "tls interference",
			lookup: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
			},
			dial: func(context.Context, string) error { return nil },
			want: "[1010] ERR_API_TLS_INTERFERENCE",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldLookup := lookupIPAddr
			oldDial := dialAddress
			lookupIPAddr = tc.lookup
			dialAddress = tc.dial
			t.Cleanup(func() {
				lookupIPAddr = oldLookup
				dialAddress = oldDial
			})
			got := diagnoseFetch("https://api.example/vpn", []string{"https: timeout"})
			if !strings.Contains(got, tc.want) {
				t.Fatalf("diagnoseFetch = %q, want %q", got, tc.want)
			}
		})
	}
}

func startSOCKSRelay(t *testing.T) (*url.URL, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		errCh <- handleSOCKSRelay(conn)
	}()
	t.Cleanup(func() {
		select {
		case err := <-errCh:
			if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
				t.Errorf("SOCKS relay: %v", err)
			}
		default:
		}
	})
	proxyURL, err := url.Parse("socks5://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return proxyURL, func() { _ = listener.Close() }
}

func handleSOCKSRelay(client net.Conn) error {
	defer client.Close()
	var greeting [2]byte
	if _, err := io.ReadFull(client, greeting[:]); err != nil {
		return err
	}
	if greeting[0] != 0x05 {
		return fmt.Errorf("unexpected socks version %d", greeting[0])
	}
	methods := make([]byte, greeting[1])
	if _, err := io.ReadFull(client, methods); err != nil {
		return err
	}
	if _, err := client.Write([]byte{0x05, 0x00}); err != nil {
		return err
	}
	host, port, err := readSOCKSConnectRequest(client)
	if err != nil {
		return err
	}
	upstream, err := net.Dial("tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	if err != nil {
		return err
	}
	defer upstream.Close()
	reply := []byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	if _, err := client.Write(reply); err != nil {
		return err
	}
	done := make(chan struct{}, 1)
	go func() {
		_, _ = io.Copy(upstream, client)
		_ = upstream.Close()
		done <- struct{}{}
	}()
	_, _ = io.Copy(client, upstream)
	<-done
	return nil
}

func readSOCKSConnectRequest(conn net.Conn) (string, int, error) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return "", 0, err
	}
	if header[0] != 0x05 || header[1] != 0x01 {
		return "", 0, fmt.Errorf("unexpected socks command")
	}
	var host string
	switch header[3] {
	case 0x01:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", 0, err
		}
		host = net.IP(addr).String()
	case 0x03:
		var length [1]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			return "", 0, err
		}
		addr := make([]byte, length[0])
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", 0, err
		}
		host = string(addr)
	case 0x04:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", 0, err
		}
		host = net.IP(addr).String()
	default:
		return "", 0, fmt.Errorf("unsupported address type %d", header[3])
	}
	var portBytes [2]byte
	if _, err := io.ReadFull(conn, portBytes[:]); err != nil {
		return "", 0, err
	}
	return host, int(binary.BigEndian.Uint16(portBytes[:])), nil
}

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"OPENVPN_UPSTREAM_SOCKS", "OPENVPN_UPSTREAM_HTTP", "https_proxy", "HTTPS_PROXY", "http_proxy", "HTTP_PROXY"} {
		t.Setenv(name, "")
	}
}
