## ADDED Requirements

### Requirement: Numeric Diagnostic Error Codes
The Go runtime SHALL use a structured numeric error code system for diagnostics equivalent to the Python runtime.

#### Scenario: Error code constants are defined
- **WHEN** the diagnostics package is imported
- **THEN** it SHALL expose numeric constants for API errors (1006-1010), OpenVPN errors (2001-2010), and local obstruction errors (3001-3008)

#### Scenario: Error codes are included in log and API responses
- **WHEN** a diagnostic check produces an error
- **THEN** the error message SHALL include both the numeric code and the tag string (e.g., `[1006] ERR_LOCAL_DNS_BROKEN`)

### Requirement: Active API Failure Diagnosis
The Go runtime SHALL perform active network probes to distinguish between different API failure modes.

#### Scenario: DNS completely broken
- **WHEN** DNS resolution fails for all tested domains AND IP connectivity to 8.8.8.8/1.1.1.1 also fails
- **THEN** the runtime SHALL report error code 1006 (`ERR_LOCAL_DNS_BROKEN`)

#### Scenario: API domain blocked but DNS works
- **WHEN** DNS resolution fails for the VPNGate API domain BUT succeeds for other domains (8.8.8.8, 1.1.1.1)
- **THEN** the runtime SHALL report error code 1007 (`ERR_API_DOMAIN_BLOCKED`)

#### Scenario: API IP blocked but network works
- **WHEN** TCP connection to the VPNGate API fails BUT TCP to external IPs (8.8.8.8:443, 1.1.1.1:443) succeeds
- **THEN** the runtime SHALL report error code 1008 (`ERR_API_IP_BLOCKED_OR_DOWN`)

#### Scenario: VPS completely offline
- **WHEN** all TCP connection tests fail on both IPv4 and IPv6
- **THEN** the runtime SHALL report error code 1009 (`ERR_VPS_OUTBOUND_BLOCKED`)

#### Scenario: TLS interference
- **WHEN** TCP connection to the API succeeds but the HTTPS request times out
- **THEN** the runtime SHALL report error code 1010 (`ERR_API_TLS_INTERFERENCE`)

### Requirement: OpenVPN Diagnostic Completeness
The Go runtime SHALL detect all Python-era OpenVPN failure patterns.

#### Scenario: Node unreachable
- **WHEN** OpenVPN logs contain connection timeout or connection refused patterns
- **THEN** the runtime SHALL report error code 2004 (`ERR_OVPN_NODE_UNREACHABLE`)

#### Scenario: Unknown OpenVPN error
- **WHEN** OpenVPN fails with an unrecognized error pattern
- **THEN** the runtime SHALL report error code 2010 (`ERR_OVPN_UNKNOWN`) as a catch-all

### Requirement: Local Obstruction Detection
The Go runtime SHALL check for local system conditions that block VPN operation.

#### Scenario: IPv4 forwarding disabled
- **WHEN** `/proc/sys/net/ipv4/ip_forward` contains `0`
- **THEN** the runtime SHALL report error code 3001 (`ERR_ROUTE_FORWARD_DISABLED`) in diagnostics

#### Scenario: Firewall blocking
- **WHEN** UFW is active without a port rule, firewalld is active, or iptables OUTPUT/FORWARD default policy is DROP
- **THEN** the runtime SHALL report error code 3007 (`ERR_FIREWALL_BLOCKING_FORWARD`) in diagnostics

#### Scenario: Strict rp_filter
- **WHEN** `rp_filter` is set to 1 (strict) on the tunnel interface
- **THEN** the runtime SHALL report error code 3008 (`ERR_ROUTE_RP_FILTER_STRICT`) in diagnostics
