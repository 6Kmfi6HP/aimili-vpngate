## Context

The Go runtime migration (`migrate-to-go-docker-release`) and the first parity pass (`restore-go-runtime-parity`) delivered a working Go service with OpenVPN management, proxy server, web UI, and API endpoints. However, sub-agent verification against the Python source reveals that several features are either absent or only partially implemented. This change addresses the remaining gaps in four focused capability areas.

## Goals / Non-Goals

**Goals:**

- Populate node metadata fields (`Owner`, `ASN`, `ASName`, `Location`, `IPType`, `Quality`) via IP enrichment so the UI can display them.
- Restore Web UI interactivity: search, filter, pagination, country translations, latency color coding, auto-refresh, connection polling, log export.
- Provide structured diagnostic error codes for API, OpenVPN, and local obstruction failures.
- Add operational hygiene: log rotation, OpenVPN version detection, orphan process cleanup, route retry, uptime tracking.

**Non-Goals:**

- Do not modify host DNS configuration.
- Do not add advertising/donation sections to the UI.
- Do not require pixel-perfect Python CSS parity.

## Decisions

### Decision: IP enrichment as a separate internal package

Create `internal/enrichment/` with its own HTTP client, JSON cache file (`ip_cache.json`), and 7-day TTL. Called from `RefreshNodes()` and `TestNodes()` after node data is available. The `ip-api.com` batch endpoint accepts up to 100 IPs per request.

Rationale: Keeps enrichment logic isolated from the VPNGate API client. Cache file lives alongside `nodes.json` in the data directory.

Alternative considered: Inline enrichment in `vpngate.go`. Rejected because it mixes concerns and makes testing harder.

### Decision: IP enrichment is opt-in via environment variable

Add `AIMILIVPN_IP_ENRICHMENT` environment variable (default: `true`). When `false`, enrichment is skipped entirely. This addresses the privacy concern of sending node IPs to a third-party API.

Rationale: Some users may not want their node IPs sent to `ip-api.com`. Making it opt-in with a default-on preserves the Python behavior while allowing privacy-conscious users to disable it.

### Decision: UI features implemented as incremental HTML/JS expansion

Expand the `indexHTML` constant in `web/server.go` with the missing JavaScript functions and HTML elements. Keep everything inline (no external files) to match the Python approach.

Rationale: The Python version embeds ~2300 lines of HTML/CSS/JS inline. The Go version currently has ~30 lines. We need to expand significantly but can do so incrementally by feature.

Alternative considered: Serve static files from disk. Rejected because it breaks the single-binary deployment model.

### Decision: Diagnostic error codes as a shared package

Create `internal/diagnostics/codes.go` defining numeric error code constants (1006-1010, 2001-2010, 3001-3008) and tag strings. Both `vpngate.go` and `openvpn.go` reference these constants instead of ad-hoc strings.

Rationale: Centralizes the error code taxonomy. Makes it possible to display user-facing error messages from codes.

### Decision: Log cleanup runs in the maintain loop

Add log cleanup to the existing `maintainLoop` goroutine, rate-limited to once per hour. Scans `logs/` directory, parses dates from `YYYY-MM-DD.json` filenames, deletes files older than 3 days.

Rationale: Reuses the existing periodic loop. Matches Python's `cleanup_old_logs()` behavior exactly.

### Decision: Policy routing retry with exponential backoff

Add 3-attempt retry with 1-second delays to `setupPolicyRouting()` in `network.go`. Matches Python's retry behavior.

Rationale: The tun device may not be fully ready immediately after OpenVPN reports initialization complete. A brief retry window handles this race condition.

## Risks / Trade-offs

- [Third-party API dependency] -> IP enrichment depends on `ip-api.com` availability. Cache mitigates repeated failures. Opt-out env var allows disabling entirely.
- [UI expansion increases binary size] -> Inline HTML/JS will grow from ~4KB to ~30KB+. Acceptable for a single binary.
- [Diagnostic probing adds latency] -> Active network probes (DNS, TCP to external IPs) add seconds to failure reporting. Only run on failure paths, not happy paths.
- [Log cleanup may delete logs user wants] -> 3-day retention matches Python behavior. Users can increase by modifying the code or backing up logs externally.
