package enrichment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/6Kmfi6HP/aimili-vpngate/internal/vpngate"
)

const (
	cacheFileName = "ip_cache.json"
	cacheTTL      = 7 * 24 * time.Hour
	batchSize     = 100
)

type cacheEntry struct {
	Owner    string  `json:"owner"`
	ASN      string  `json:"asn"`
	ASName   string  `json:"as_name"`
	Location string  `json:"location"`
	IPType   string  `json:"ip_type"`
	Quality  string  `json:"quality"`
	CachedAt float64 `json:"cached_at"`
}

type apiResponse struct {
	Status     string `json:"status"`
	Query      string `json:"query"`
	Country    string `json:"country"`
	RegionName string `json:"regionName"`
	City       string `json:"city"`
	ISP        string `json:"isp"`
	Org        string `json:"org"`
	AS         string `json:"as"`
	ASName     string `json:"asname"`
	Proxy      bool   `json:"proxy"`
	Hosting    bool   `json:"hosting"`
	Mobile     bool   `json:"mobile"`
}

var (
	cacheMu   sync.Mutex
	cachePath string
	enabled   = true
	endpoint  = "http://ip-api.com/batch?lang=zh-CN&fields=status,message,query,country,regionName,city,isp,org,as,asname,proxy,hosting,mobile"
	client    = &http.Client{Timeout: 15 * time.Second}
)

func Configure(dataDir string, isEnabled bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	enabled = isEnabled
	if dataDir == "" {
		cachePath = cacheFileName
		return
	}
	cachePath = filepath.Join(dataDir, cacheFileName)
}

func CacheFile(dataDir string) string {
	return filepath.Join(dataDir, cacheFileName)
}

func EnrichNodes(nodes []vpngate.Node) error {
	cacheMu.Lock()
	if !enabled {
		cacheMu.Unlock()
		return nil
	}
	path := cachePath
	if path == "" {
		path = cacheFileName
	}
	cache := loadCacheLocked(path)
	now := time.Now()
	var toQuery []string
	seen := map[string]bool{}
	for i := range nodes {
		ip := nodeIP(nodes[i])
		if ip == "" {
			continue
		}
		if entry, ok := cache[ip]; ok && now.Sub(cachedTime(entry)) < cacheTTL {
			applyEntry(&nodes[i], entry)
			continue
		}
		if !seen[ip] {
			seen[ip] = true
			toQuery = append(toQuery, ip)
		}
	}
	cacheMu.Unlock()

	if len(toQuery) == 0 {
		return nil
	}

	var errs []error
	newEntries := map[string]cacheEntry{}
	for start := 0; start < len(toQuery); start += batchSize {
		end := start + batchSize
		if end > len(toQuery) {
			end = len(toQuery)
		}
		entries, err := fetchBatch(toQuery[start:end], now)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for ip, entry := range entries {
			newEntries[ip] = entry
		}
	}

	if len(newEntries) > 0 {
		cacheMu.Lock()
		cache = loadCacheLocked(path)
		for ip, entry := range newEntries {
			cache[ip] = entry
		}
		err := saveCacheLocked(path, cache)
		cacheMu.Unlock()
		if err != nil {
			errs = append(errs, err)
		}
		for i := range nodes {
			if entry, ok := newEntries[nodeIP(nodes[i])]; ok {
				applyEntry(&nodes[i], entry)
			}
		}
	}

	return errors.Join(errs...)
}

func loadCacheLocked(path string) map[string]cacheEntry {
	out := map[string]cacheEntry{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func saveCacheLocked(path string, cache map[string]cacheEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

func fetchBatch(ips []string, now time.Time) (map[string]cacheEntry, error) {
	payload, err := json.Marshal(ips)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "aimilivpn-go/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ip enrichment status %s", resp.Status)
	}
	var items []apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}
	out := map[string]cacheEntry{}
	for _, item := range items {
		if item.Status != "success" || item.Query == "" {
			continue
		}
		out[item.Query] = cacheEntry{
			Owner:    firstNonEmpty(item.Org, item.ISP),
			ASN:      item.AS,
			ASName:   item.ASName,
			Location: joinLocation(item.Country, item.RegionName, item.City),
			IPType:   ipType(item),
			Quality:  quality(item),
			CachedAt: float64(now.Unix()),
		}
	}
	return out, nil
}

func nodeIP(node vpngate.Node) string {
	if node.IP != "" {
		return node.IP
	}
	return node.RemoteHost
}

func applyEntry(node *vpngate.Node, entry cacheEntry) {
	node.Owner = entry.Owner
	node.ASN = entry.ASN
	node.ASName = entry.ASName
	node.Location = entry.Location
	node.IPType = entry.IPType
	node.Quality = entry.Quality
}

func cachedTime(entry cacheEntry) time.Time {
	return time.Unix(int64(entry.CachedAt), 0)
}

func joinLocation(parts ...string) string {
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func ipType(item apiResponse) string {
	switch {
	case item.Mobile:
		return "mobile"
	case item.Proxy:
		return "proxy"
	case item.Hosting:
		return "hosting"
	default:
		return "residential"
	}
}

func quality(item apiResponse) string {
	switch {
	case item.Proxy:
		return "proxy"
	case item.Hosting:
		return "datacenter"
	case item.Mobile:
		return "mobile"
	default:
		return "normal"
	}
}
