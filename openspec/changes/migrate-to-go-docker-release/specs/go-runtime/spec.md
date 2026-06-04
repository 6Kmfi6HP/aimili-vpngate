## ADDED Requirements

### Requirement: Go Service Runtime
The system SHALL provide a Go service binary that is the primary AimiliVPN runtime and does not require Python to start, manage VPNGate nodes, run the management UI, run the local proxy, or manage OpenVPN.

#### Scenario: Service starts without Python runtime
- **WHEN** the Go binary is installed on a supported Linux host with required system networking tools
- **THEN** it starts the manager, web UI, background workers, and local proxy without invoking Python

#### Scenario: Missing runtime dependency is reported
- **WHEN** a required external runtime dependency such as `openvpn` or a Linux networking command is missing
- **THEN** the service reports a clear diagnostic and does not fail silently

### Requirement: Current Behavior Parity
The Go runtime SHALL preserve the existing AimiliVPN user-facing behavior for VPNGate fetching, concurrent node checks, OpenVPN connection management, auto mode, fixed country mode, fixed IP mode, state visibility, logs, management UI, and local HTTP/SOCKS5 proxy service.

#### Scenario: User connects through an automatically selected node
- **WHEN** a user starts the service in automatic mode and VPNGate nodes are reachable
- **THEN** the service fetches nodes, tests candidates, starts OpenVPN for a healthy node, and exposes proxy traffic through the active tunnel

#### Scenario: User selects a fixed node
- **WHEN** a user selects a fixed country or fixed IP mode from the management interface
- **THEN** the Go runtime keeps the selected route policy and only changes nodes according to that policy

### Requirement: Compatible Configuration And State
The Go runtime SHALL preserve compatible defaults and environment configuration for data directory, fetch intervals, scan limits, OpenVPN command/authentication, management UI host/port, and local proxy host/port.

#### Scenario: Existing environment variables are used
- **WHEN** a deployment sets existing environment variables such as `VPNGATE_DATA_DIR`, `LOCAL_PROXY_HOST`, `LOCAL_PROXY_PORT`, `UI_HOST`, or `UI_PORT`
- **THEN** the Go runtime applies those values with behavior compatible with the previous runtime

#### Scenario: Existing data directory is migrated
- **WHEN** an existing `vpngate_data` directory is present during migration
- **THEN** the service preserves user authentication, state, node cache, logs, and generated configs where their formats remain valid

### Requirement: Secure Network Binding Defaults
The Go runtime SHALL keep the local proxy bound to `127.0.0.1:7928` by default for native installs and SHALL require explicit configuration before accepting proxy traffic from public interfaces.

#### Scenario: Native default proxy binding
- **WHEN** the service starts without proxy binding overrides in a native install
- **THEN** the local proxy listens only on `127.0.0.1:7928`

#### Scenario: Public proxy binding is explicit
- **WHEN** an operator configures the proxy to listen on a non-loopback address
- **THEN** the service accepts the setting only through explicit configuration and the documentation identifies the exposure risk

### Requirement: OpenVPN Failure Handling
The Go runtime SHALL surface OpenVPN, tun/tap, DNS, route, and firewall failures through logs, status, and management diagnostics.

#### Scenario: Tun device is unavailable
- **WHEN** `/dev/net/tun` is missing or inaccessible
- **THEN** the service reports the tun/tap failure and avoids reporting the VPN as connected

#### Scenario: OpenVPN process exits unexpectedly
- **WHEN** the active OpenVPN process exits unexpectedly while auto mode is enabled
- **THEN** the service records the failure and attempts recovery according to the configured node selection policy
