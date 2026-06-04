## Context

The previous Python runtime used three cooperating modules: `vpngate_manager.py` for lifecycle, UI, API, state, and OpenVPN control; `proxy_server.py` for HTTP/SOCKS5 proxy traffic bound to `tun0`; and `vpn_utils.py` for upstream proxy detection, latency checks, route helpers, DNS/API diagnostics, and IP metadata. The current Go runtime has the same broad package boundaries under `internal/`, but sub-agent review found several behavior gaps that are user-visible or safety-sensitive.

This change treats parity as observable behavior rather than line-by-line translation. The Go service may keep a simpler implementation, but it must not claim success for settings, node checks, OpenVPN readiness, proxy health, or gateway status unless the old Python workflow would have had a meaningful success signal.

## Goals / Non-Goals

**Goals:**

- Restore safe OpenVPN startup behavior for active connections and node probes.
- Replace TCP-only final node validation with bounded OpenVPN readiness validation for selected candidates.
- Restore Linux route setup and cleanup behavior needed by the local proxy gateway.
- Verify proxy health through real local-proxy egress, not only listener reachability.
- Restore management API and UI workflows that existing users rely on.
- Preserve compatible state, log, node, auth, and CLI config behavior.
- Add tests and smoke checks around the exact parity gaps discovered by review.

**Non-Goals:**

- Do not rework Docker image publishing, release automation, or installer mode selection.
- Do not mutate host DNS configuration such as `/etc/resolv.conf`.
- Do not require full third-party IP enrichment before node actions work.
- Do not guarantee VPN behavior outside Linux hosts with OpenVPN and required networking privileges.
- Do not require exact Python visual styling where the Go UI provides the same workflow clearly.

## Decisions

### Decision: Define parity around two capability areas

This change uses `go-runtime-networking-parity` for OpenVPN, node validation, routing, proxy health, recovery, and API fetch behavior. It uses `management-api-ui-parity` for HTTP API, UI, state/log, node shape, and CLI compatibility.

Rationale: agents found two independent clusters of high-value work. Keeping them in one change preserves end-to-end acceptance while allowing implementation tasks to be split cleanly.

Alternative considered: separate networking and UI proposals. Rejected for now because many user-visible API endpoints, such as `test_proxy`, `gateway_status`, and node testing, are the management surface for networking behavior.

### Decision: Use OpenVPN readiness as the final node availability signal

Go may keep TCP dialing as a cheap prefilter, but a node must not be reported as available based only on TCP reachability. Final availability for selected candidates should come from a bounded OpenVPN startup probe with a disposable tun device and `route-nopull`.

Rationale: VPNGate includes UDP and TCP configs, and a reachable TCP port does not prove OpenVPN authentication, TLS negotiation, pushed option handling, or tun creation. Python used OpenVPN readiness as the real check.

Alternative considered: keep TCP-only checks for speed. Rejected because it is the root cause of false positives and false negatives in the migrated runtime.

### Decision: Treat route and proxy health as best-effort but observable

The Go runtime should configure policy routing and loose `rp_filter` on Linux when possible, clean those rules on disconnect or replacement, and surface failures through status/logs. It should still rely on tun-bound proxy dialing where available, but route setup failures must not be invisible.

Rationale: route changes are security-sensitive and host-dependent. Best-effort with clear diagnostics is safer than either silent failure or hard failure in every constrained container.

Alternative considered: rely only on `SO_BINDTODEVICE`. Rejected because the Python runtime also used explicit route table cleanup/setup, and operational diagnostics need to know when host route assumptions are broken.

### Decision: Validate management changes before persisting them

Settings endpoints should reject invalid ports, conflicting ports, invalid secret suffixes, and unknown route modes before saving. Listener-affecting changes should report `restart_needed` and either trigger a controlled service restart or clearly state that the next service restart is required.

Rationale: the current Go API can persist settings that are not applied to existing listeners, which makes the management UI misleading.

Alternative considered: hot-rebind web and proxy listeners immediately. Deferred because it is more complex than matching Python's restart-triggered behavior and must be designed carefully around active connections.

### Decision: Preserve compatibility fields even when values are empty

Go nodes and state should include Python-era fields needed by the UI and scripts, including routing settings in state, node metadata fields, latency/probe timestamps, and structured log timestamps. Unknown JSON fields in config files should be preserved by CLI writes.

Rationale: compatibility is partly schema shape. Empty but stable fields are less disruptive than missing fields.

Alternative considered: expose only the smaller Go-native shape. Rejected because it breaks existing UI workflows and downstream scripts for little benefit.

## Risks / Trade-offs

- [OpenVPN probing is expensive] -> Limit final OpenVPN probes to selected candidates, cap concurrency, use short timeouts, and keep TCP checks as a prefilter.
- [Root or container capability requirements vary] -> Surface missing tun, route, bind, and OpenVPN permissions in status/logs and do not report false success.
- [Policy routing may conflict with host networking] -> Make cleanup idempotent and scoped to the service-owned table/rules.
- [Proxy egress checks can fail because public IP services are down] -> Try multiple endpoints and report the tested endpoint and failure category.
- [UI parity can expand indefinitely] -> Restore workflows first; defer cosmetic parity and full third-party IP enrichment.
- [Existing migration tasks claim completion] -> Add executable tests and mark this change as a hardening/parity correction rather than another broad migration.

## Migration Plan

1. Add failing tests around the discovered Go parity gaps before implementation where practical.
2. Implement networking parity in small slices: OpenVPN command builder, probe manager, route manager, proxy egress checker, and API fetch diagnostics.
3. Implement management parity in small slices: API behavior, state/log schema, CLI config preservation, and UI workflow controls.
4. Update docs and smoke-test notes to describe actual Go behavior and deferred parity items.
5. Verify with Go unit tests, Go vet/build, installer syntax, OpenSpec validation, and native smoke checks for OpenVPN, proxy egress, settings restart, and UI/API workflows.

Rollback strategy: keep the previous Go behavior behind no persistent data migration. If route or probe changes cause operational regressions, disabling auto-connect and restoring previous service binaries should leave `vpngate_data` readable.

## Open Questions

- Should listener-affecting settings immediately terminate the Go process like Python, or return `restart_needed` and leave restart to `ml restart`?
- Should IP metadata enrichment remain optional, config-gated, or excluded from this parity change?
- Should `gateway_status` preserve Python's six-service model exactly, or expose Go worker names while keeping equivalent health categories?
