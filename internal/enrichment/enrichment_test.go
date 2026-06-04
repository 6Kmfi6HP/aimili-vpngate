package enrichment

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/6Kmfi6HP/aimili-vpngate/internal/vpngate"
)

func TestEnrichNodesUsesMockedBatchResponseAndCache(t *testing.T) {
	dir := t.TempDir()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		var ips []string
		if err := json.NewDecoder(r.Body).Decode(&ips); err != nil {
			t.Fatal(err)
		}
		if len(ips) != 1 || ips[0] != "203.0.113.10" {
			t.Fatalf("unexpected payload: %#v", ips)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"status":     "success",
			"query":      "203.0.113.10",
			"country":    "Japan",
			"regionName": "Tokyo",
			"city":       "Chiyoda",
			"isp":        "Example ISP",
			"org":        "Example Org",
			"as":         "AS64500 Example",
			"asname":     "EXAMPLE-AS",
			"hosting":    true,
		}})
	}))
	defer server.Close()
	configureForTest(t, dir, server)

	nodes := []vpngate.Node{{ID: "node", IP: "203.0.113.10"}}
	if err := EnrichNodes(nodes); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d", requests)
	}
	if nodes[0].Owner != "Example Org" || nodes[0].ASN != "AS64500 Example" || nodes[0].ASName != "EXAMPLE-AS" {
		t.Fatalf("node not enriched: %#v", nodes[0])
	}
	if nodes[0].Location != "Japan Tokyo Chiyoda" || nodes[0].IPType != "hosting" || nodes[0].Quality != "datacenter" {
		t.Fatalf("unexpected enrichment fields: %#v", nodes[0])
	}

	cached := []vpngate.Node{{ID: "node-2", IP: "203.0.113.10"}}
	if err := EnrichNodes(cached); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("cache miss caused request count %d", requests)
	}
	if cached[0].Owner != "Example Org" {
		t.Fatalf("cached node not enriched: %#v", cached[0])
	}
}

func TestEnrichNodesChunksRequestsAtOneHundredIPs(t *testing.T) {
	dir := t.TempDir()
	var batchSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ips []string
		if err := json.NewDecoder(r.Body).Decode(&ips); err != nil {
			t.Fatal(err)
		}
		batchSizes = append(batchSizes, len(ips))
		items := make([]map[string]any, 0, len(ips))
		for _, ip := range ips {
			items = append(items, map[string]any{
				"status":  "success",
				"query":   ip,
				"country": "United States",
				"isp":     "ISP " + ip,
			})
		}
		_ = json.NewEncoder(w).Encode(items)
	}))
	defer server.Close()
	configureForTest(t, dir, server)

	nodes := make([]vpngate.Node, 101)
	for i := range nodes {
		nodes[i] = vpngate.Node{ID: fmt.Sprintf("node-%d", i), IP: fmt.Sprintf("198.51.100.%d", i)}
	}
	if err := EnrichNodes(nodes); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(batchSizes) != "[100 1]" {
		t.Fatalf("batch sizes = %#v", batchSizes)
	}
	if nodes[100].Owner == "" {
		t.Fatalf("last node not enriched: %#v", nodes[100])
	}
}

func TestEnrichNodesSkipsExpiredCacheAndUsesFreshCache(t *testing.T) {
	dir := t.TempDir()
	Configure(dir, true)
	cachePath = filepath.Join(dir, cacheFileName)
	freshIP := "192.0.2.1"
	expiredIP := "192.0.2.2"
	cache := map[string]cacheEntry{
		freshIP:   {Owner: "Fresh", CachedAt: float64(time.Now().Add(-time.Hour).Unix())},
		expiredIP: {Owner: "Expired", CachedAt: float64(time.Now().Add(-8 * 24 * time.Hour).Unix())},
	}
	if err := saveCacheLocked(cachePath, cache); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ips []string
		if err := json.NewDecoder(r.Body).Decode(&ips); err != nil {
			t.Fatal(err)
		}
		if len(ips) != 1 || ips[0] != expiredIP {
			t.Fatalf("unexpected refreshed IPs: %#v", ips)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"status": "success",
			"query":  expiredIP,
			"org":    "Refreshed",
		}})
	}))
	defer server.Close()
	configureForTest(t, dir, server)

	nodes := []vpngate.Node{{IP: freshIP}, {IP: expiredIP}}
	if err := EnrichNodes(nodes); err != nil {
		t.Fatal(err)
	}
	if nodes[0].Owner != "Fresh" || nodes[1].Owner != "Refreshed" {
		t.Fatalf("unexpected nodes: %#v", nodes)
	}
}

func configureForTest(t *testing.T, dir string, server *httptest.Server) {
	t.Helper()
	oldEndpoint := endpoint
	oldClient := client
	oldCachePath := cachePath
	oldEnabled := enabled
	endpoint = server.URL
	client = server.Client()
	cachePath = filepath.Join(dir, cacheFileName)
	enabled = true
	t.Cleanup(func() {
		endpoint = oldEndpoint
		client = oldClient
		cachePath = oldCachePath
		enabled = oldEnabled
		_ = os.RemoveAll(dir)
	})
}
