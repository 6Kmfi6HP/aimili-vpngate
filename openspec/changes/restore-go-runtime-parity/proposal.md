## Why

The Go migration is structurally present, but sub-agent review found that several Python-era behaviors are either missing, stubbed, or only partially implemented in the current Go runtime. This matters now because the gaps affect real connection success, safe routing, management UI/API trustworthiness, and compatibility for existing `vpngate_data` and `ml` users.

## What Changes

- Restore high-impact Go runtime parity for OpenVPN command construction, node validation, Linux route setup/cleanup, proxy egress health checks, and API fetch diagnostics.
- Replace placeholder or weakened management API behavior with observable Python-compatible workflows for node testing, proxy testing, gateway status, settings validation, state payloads, logs, and CLI config preservation.
- Restore a usable management UI workflow for node list/actions, route mode settings, credentials/settings updates, gateway self-check, logs, and active connection status.
- Preserve compatible node/state/log fields used by the Python UI and existing operators, while allowing Go-specific fields to remain additive.
- Add focused tests and smoke documentation so future changes cannot mark parity-sensitive behavior complete without executable coverage.
- Defer low-value or high-risk parity items such as mutating `/etc/resolv.conf`, full visual polish, broad non-Linux VPN support, and full third-party IP enrichment unless explicitly enabled.

## Capabilities

### New Capabilities

- `go-runtime-networking-parity`: Go runtime networking behavior required for safe OpenVPN startup, real node validation, route cleanup, proxy egress checks, auto-recovery, and API fetch diagnostics.
- `management-api-ui-parity`: Management API, UI, state/log, node metadata, and CLI config behavior required to match existing Python-era operational workflows.

### Modified Capabilities

- None. Canonical `openspec/specs/` is currently empty because the earlier migration change has not been archived into main specs.

## Impact

- User impact: Go deployments should behave like the previous Python service for connection selection, node testing, settings changes, gateway health, logs, and web management actions.
- Operational impact: OpenVPN checks and route/proxy health checks may consume more time and privileges than the current TCP-only Go probe, so concurrency and timeout limits must be explicit.
- Security impact: proxy binding remains local-only by default; route, tun/tap, DNS, auth files, secret paths, and listener restarts remain security-sensitive and must be validated before reporting success.
- Compatibility impact: existing `ui_auth.json`, `state.json`, `nodes.json`, structured logs, config files, and `ml` workflows must remain readable and must not lose unknown fields during updates.
- Documentation impact: migration/checklist docs should clearly distinguish implemented parity from deferred behavior and should include native smoke checks for OpenVPN, proxy egress, settings restart, and API/UI workflows.

### Non-Goals

- Do not redesign Docker, release automation, or native installer flows in this change.
- Do not replace OpenVPN or change the VPNGate upstream protocol.
- Do not make the local proxy or management UI public by default.
- Do not write public DNS servers into `/etc/resolv.conf`; diagnostics and fallback attempts may be added without host DNS mutation.
- Do not require exact line-by-line Python implementation parity where an equivalent observable Go behavior is safer or simpler.
