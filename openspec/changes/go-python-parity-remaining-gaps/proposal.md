## Why

Independent sub-agent verification confirms that the Go runtime is still missing several Python-era capabilities that were either not covered by the `restore-go-runtime-parity` change or were marked complete without being fully implemented. These gaps affect user-visible functionality (IP metadata in node table, UI search/filter/pagination), operational resilience (log disk growth, orphaned processes, DNS self-healing, route setup retries), and diagnostic accuracy (detailed error codes for API/OpenVPN/firewall failures).

The `restore-go-runtime-parity` proposal explicitly deferred IP enrichment and DNS mutation as non-goals and did not address log cleanup, OpenVPN version detection, startup process cleanup, policy routing retries, or the majority of Web UI JavaScript features. This proposal fills those specific, verified gaps.

## What Changes

- Add IP enrichment via `ip-api.com/batch` with 7-day JSON cache, populating `Owner`, `ASN`, `ASName`, `Location`, `IPType`, `Quality` on Go nodes.
- Expand the Web UI to include search/filter, pagination, country filter dropdown, latency color coding, auto-refresh polling, connection status polling, log export/copy, country name translations, active connection card, and node sorting.
- Add comprehensive network diagnostics with numeric error codes for API failures (DNS broken, domain blocked, IP blocked, TLS interference), OpenVPN failures (node unreachable, unknown errors), and local obstructions (IP forwarding disabled, firewall blocking, strict rp_filter).
- Add log rotation to delete JSON log files older than 3 days.
- Add OpenVPN version detection to adapt cipher configuration flags.
- Add startup cleanup of orphaned OpenVPN processes from prior runs.
- Add retry logic (3 attempts) to policy routing setup.
- Add server uptime tracking with startup grace periods for heartbeat checks.

## Capabilities

### New Capabilities

- `ip-enrichment`: Batch IP metadata lookup, caching, and UI display for node table enrichment columns.
- `web-ui-feature-parity`: Search, filter, pagination, translations, polling, log management, and visual feedback features in the Go management UI.
- `diagnostics-robustness`: Numeric error codes, active network probing, firewall detection, and structured diagnostic reporting.
- `runtime-hygiene`: Log rotation, OpenVPN version detection, orphan process cleanup, route retry, and uptime tracking.

### Modified Capabilities

- None. These are additive to the existing `go-runtime-networking-parity` and `management-api-ui-parity` capabilities.

## Impact

- User impact: Node table will show ASN/ISP/location/quality columns. UI will be searchable, filterable, and paginated. Logs won't fill disk. Error messages will be more specific.
- Operational impact: IP enrichment adds outbound HTTP calls to `ip-api.com` (batch of 100, cached 7 days). Log cleanup runs hourly. Startup process cleanup runs once. These are low-overhead.
- Security impact: IP enrichment sends node IPs to a third-party API. This should be configurable (opt-in/opt-out). No other security-sensitive changes.
- Compatibility impact: All changes are additive. Existing `nodes.json`, `state.json`, `ui_auth.json` remain compatible. New fields are populated where applicable.

### Non-Goals

- Do not modify `/etc/resolv.conf` (DNS auto-fix remains deferred per `restore-go-runtime-parity`).
- Do not redesign the proxy server protocol handling.
- Do not add VPS promotion/advertising sections or donation information to the UI.
- Do not require exact Python visual styling parity (glassmorphism, animations) where functional parity is achieved.
