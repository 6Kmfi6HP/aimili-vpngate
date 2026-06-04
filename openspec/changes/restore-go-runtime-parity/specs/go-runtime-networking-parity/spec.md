## ADDED Requirements

### Requirement: Safe OpenVPN Command Construction
The Go runtime SHALL construct OpenVPN commands with safe routing and compatibility flags equivalent to the Python runtime for both active connections and node probes.

#### Scenario: Active OpenVPN start uses safe flags
- **WHEN** the Go runtime starts OpenVPN for an active node
- **THEN** it SHALL pass an explicit tun device, tun device type, auth file, auth nocache, bounded retry/connect timeout settings, IPv6 route/ifconfig pull filters, route delay, cipher compatibility options, and route-nopull behavior consistent with the managed proxy gateway design

#### Scenario: TCP node config uses configured upstream proxy
- **WHEN** a selected OpenVPN config uses TCP and an upstream SOCKS or HTTP proxy environment setting is configured
- **THEN** the OpenVPN command SHALL include the corresponding upstream proxy option

#### Scenario: OpenVPN readiness updates status
- **WHEN** OpenVPN emits startup progress, readiness, authentication, TLS, DNS, tun/tap, option, or timeout signals
- **THEN** the Go runtime SHALL update state and logs with a user-facing status or diagnostic instead of only returning a raw process error

### Requirement: Real Node Availability Validation
The Go runtime SHALL use OpenVPN readiness, not TCP reachability alone, as the final availability signal for selected VPNGate candidates.

#### Scenario: Candidate is validated through OpenVPN
- **WHEN** the runtime checks a candidate selected for availability testing
- **THEN** it SHALL write a temporary config, start OpenVPN with a disposable tun device and route-nopull, wait for readiness or failure within a bounded timeout, stop the probe process, and update the node probe result

#### Scenario: TCP probe is only a prefilter
- **WHEN** a TCP dial succeeds for a candidate
- **THEN** the runtime MUST NOT mark the node as available until the OpenVPN readiness probe succeeds

#### Scenario: UDP candidate is not rejected by TCP-only logic
- **WHEN** a candidate uses UDP OpenVPN transport
- **THEN** the runtime SHALL keep it eligible for OpenVPN readiness probing instead of requiring TCP dial success as the final test

#### Scenario: Probe concurrency is bounded
- **WHEN** multiple nodes are tested from background refresh or `/api/test_nodes`
- **THEN** the runtime SHALL limit concurrent OpenVPN probes and tun device allocation to avoid exhausting host privileges or devices

### Requirement: Linux Route Setup And Cleanup
The Go runtime SHALL manage service-owned Linux route rules for active tunnels and SHALL clean them up on disconnect, replacement, or process shutdown.

#### Scenario: Route setup follows successful tunnel readiness
- **WHEN** OpenVPN reports the active tunnel is ready on Linux
- **THEN** the runtime SHALL attempt to configure a service-owned route table and rule for the tunnel interface and set loose reverse-path filtering where supported

#### Scenario: Route setup failure is visible
- **WHEN** route setup fails because `ip`, privileges, route tables, or sysctl writes are unavailable
- **THEN** the runtime SHALL record a diagnostic in state/logs and gateway status instead of silently reporting full gateway health

#### Scenario: Route cleanup is idempotent
- **WHEN** the active VPN is disconnected, replaced, or the service stops
- **THEN** the runtime SHALL remove service-owned route rules and flush the service-owned route table without deleting unrelated host routing state

### Requirement: Proxy Egress Health And Recovery
The Go runtime SHALL verify local proxy health through actual proxy egress and SHALL use those results for state, diagnostics, and recovery.

#### Scenario: Proxy test verifies egress
- **WHEN** `/api/test_proxy` is requested
- **THEN** the runtime SHALL verify listener reachability, tunnel device availability on Linux, outbound connectivity through the local proxy, observed egress IP where available, and latency or failure reason

#### Scenario: Background proxy failure triggers recovery
- **WHEN** an active node exists and repeated proxy egress checks fail
- **THEN** the runtime SHALL mark the active node failed, apply invalid-node backoff, and attempt recovery according to auto, fixed region, or fixed IP route policy

#### Scenario: Fixed IP mode does not silently move nodes
- **WHEN** fixed IP mode is enabled and the active node fails
- **THEN** the runtime SHALL retry or report failure for the selected node without automatically switching to a different IP

### Requirement: VPNGate Fetch Fallbacks And Diagnostics
The Go runtime SHALL provide non-mutating API fetch fallbacks and diagnostics comparable to the Python runtime.

#### Scenario: Upstream proxy is configured
- **WHEN** `OPENVPN_UPSTREAM_SOCKS`, `OPENVPN_UPSTREAM_HTTP`, or standard proxy environment variables are configured
- **THEN** VPNGate API fetching SHALL attempt the configured upstream proxy before falling back according to documented behavior

#### Scenario: HTTPS fetch fails
- **WHEN** the default HTTPS VPNGate API fetch fails
- **THEN** the runtime SHALL attempt configured fallback paths such as HTTPS without certificate verification and HTTP fallback where allowed, and SHALL record which attempts failed

#### Scenario: DNS or API access fails
- **WHEN** VPNGate API fetch ultimately fails because of DNS, TCP, TLS, blocked API host, or local outbound failure
- **THEN** the runtime SHALL expose a diagnostic category in state/logs so operators can distinguish local DNS, API domain, API IP, TLS, and outbound connectivity failures

#### Scenario: Host DNS is not mutated
- **WHEN** API DNS diagnostics detect a broken resolver
- **THEN** the runtime MUST NOT edit `/etc/resolv.conf` as part of this change
