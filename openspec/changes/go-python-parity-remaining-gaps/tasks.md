## 1. IP Enrichment

- [x] 1.1 Create `internal/enrichment/enrichment.go` with `EnrichNodes(nodes []vpngate.Node) error` function that batches IPs to `ip-api.com/batch`, parses responses, and populates `Owner`, `ASN`, `ASName`, `Location`, `IPType`, `Quality` fields.
- [x] 1.2 Implement JSON file cache (`ip_cache.json`) with 7-day TTL, thread-safe read/write, and chunked batch processing (100 IPs per request).
- [x] 1.3 Add `AIMILIVPN_IP_ENRICHMENT` environment variable to `config.go` (default: `true`) to allow opt-out.
- [x] 1.4 Integrate enrichment call into `app.go:RefreshNodes()` and `app.go:TestNodes()` after node data is available.
- [x] 1.5 Add enrichment tests with mocked HTTP responses.
- [x] 1.6 Update `state/store.go` to handle `ip_cache.json` in migration backup.

## 2. Web UI Feature Parity

- [x] 2.1 Add search input and `getFilteredNodes()` JavaScript function that filters by country, location, IP, ASN, ISP across all node fields.
- [x] 2.2 Add country filter dropdown with `updateCountryFilter()` that dynamically populates from node data with node counts.
- [x] 2.3 Add pagination controls (first/prev/next/last, 11 items/page) with `render()` updates.
- [x] 2.4 Add latency color coding: green (<50ms), yellow (<150ms), red (>=150ms) via CSS classes and `getLatencyClass()`.
- [x] 2.5 Add `translateCountry()` with 50+ English-to-Chinese country name mappings.
- [x] 2.6 Add `translateStatus()`, `translateQuality()`, `translateIpType()` translation functions.
- [x] 2.7 Add ASN, ISP/Owner, Location, IP Type, Quality columns to the node table.
- [x] 2.8 Add auto-refresh polling (10s interval when page visible and idle).
- [x] 2.9 Add `startConnectionPolling()` (1s interval during connection with auto-stop).
- [x] 2.10 Add active connection card with animated connecting state, latency display, and disconnect button.
- [x] 2.11 Add log export (`exportLogContent()` as .txt download) and copy-to-clipboard (`copyLogContent()`).
- [x] 2.12 Add log level filtering (All/Proxy/VPN/System).
- [x] 2.13 Add `stableSortNodes()` for score-based node sorting.
- [x] 2.14 Add disconnect confirmation dialog.
- [x] 2.15 Add GitHub and Telegram header links.
- [x] 2.16 Add gateway status modal with 3s polling and service health cards.
- [x] 2.17 Add routing mode descriptions via `handleRoutingModeChange()`.

## 3. Diagnostics And Error Codes

- [x] 3.1 Create `internal/diagnostics/codes.go` with numeric error code constants and tag strings for API (1006-1010), OpenVPN (2001-2010), and local obstruction (3001-3008) categories.
- [x] 3.2 Update `vpngate.go:diagnoseFetch()` to perform active DNS and TCP probes (8.8.8.8, 1.1.1.1, IPv6 equivalents) to distinguish DNS broken (1006), domain blocked (1007), IP blocked (1008), VPS offline (1009), and TLS interference (1010).
- [x] 3.3 Add `ERR_OVPN_NODE_UNREACHABLE` (2004) and `ERR_OVPN_UNKNOWN` (2010) patterns to `openvpn.go:diagnoseOpenVPNLine()`.
- [x] 3.4 Add IPv4 forwarding check (`/proc/sys/net/ipv4/ip_forward`) to `diagnostics.go:RuntimeChecks()`.
- [x] 3.5 Add firewall policy inspection (UFW, firewalld, iptables OUTPUT/FORWARD DROP) to `diagnostics.go:RuntimeChecks()`.
- [x] 3.6 Add rp_filter check to `diagnostics.go:RuntimeChecks()`.
- [x] 3.7 Update `web/server.go` gateway status and log endpoints to include numeric error codes in diagnostic messages.

## 4. Runtime Hygiene

- [x] 4.1 Add `CleanupOldLogs()` to `state/store.go` that deletes `logs/YYYY-MM-DD.json` files older than 3 days. Call from `maintainLoop` with hourly rate limit.
- [x] 4.2 Add `DetectOpenVPNVersion()` to `openvpn/openvpn.go` that runs `openvpn --version`, parses version number, caches result, and returns version string. Default to "2.4" on failure.
- [x] 4.3 Update `openvpn.go:BuildCommand()` to use `ncp-ciphers` for OpenVPN <2.5 and `data-ciphers` for >=2.5.
- [x] 4.4 Add `KillOrphanProcesses()` to `openvpn/openvpn.go` that runs `pkill -f openvpn.*tun0` and `pkill -f openvpn.*vpngate_data` on startup. Call from `app.go:Run()`.
- [x] 4.5 Add 3-attempt retry with 1-second delay to `network.go:setupPolicyRouting()`.
- [x] 4.6 Add `startTime time.Time` field to `App` struct. Record at `New()`. Use for heartbeat grace periods (15s, 35s) in `heartbeatStatus()`.
- [x] 4.7 Add tests for log cleanup, version detection, route retry, and uptime tracking.

## 5. Verification

- [x] 5.1 Run `gofmt ./...`, `go test ./...`, `go vet ./...`, `go build ./cmd/aimilivpn`.
- [x] 5.2 Run `openspec validate go-python-parity-remaining-gaps`.
- [x] 5.3 Verify IP enrichment populates node fields with test fixture data.
- [x] 5.4 Verify UI search, filter, pagination, and translations work in browser.
- [x] 5.5 Verify diagnostic error codes are returned for simulated failure scenarios.
- [x] 5.6 Verify log files older than 3 days are cleaned up.
- [x] 5.7 Verify orphan OpenVPN processes are killed on startup.
