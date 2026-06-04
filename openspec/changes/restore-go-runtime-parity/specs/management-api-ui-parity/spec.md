## ADDED Requirements

### Requirement: Management API Endpoint Parity
The Go management API SHALL provide observable behavior equivalent to the Python management API for node actions, proxy tests, gateway status, settings, logs, and runtime state.

#### Scenario: Batch node test updates nodes
- **WHEN** a client posts selected node IDs to `/api/test_nodes`
- **THEN** the API SHALL test those nodes, update persisted node probe fields, and return the tested node results instead of a placeholder success

#### Scenario: Single node test updates node
- **WHEN** a client posts a node ID to `/api/test_node`
- **THEN** the API SHALL test that node through the same availability path used by batch tests and return the updated node

#### Scenario: Proxy test updates state
- **WHEN** a client posts to `/api/test_proxy`
- **THEN** the API SHALL run the real proxy egress health check and update `proxy_ok`, `proxy_ip`, `proxy_latency_ms`, and `proxy_error` in runtime state

#### Scenario: Gateway status uses real checks
- **WHEN** a client gets `/api/gateway_status`
- **THEN** the API SHALL report web service, local proxy, OpenVPN core, node refresh worker, proxy health worker, and active latency worker health or equivalent Go worker categories with real status and diagnostic details

### Requirement: Settings Validation And Restart Signaling
The Go management API SHALL validate settings before persistence and SHALL accurately report whether a restart is needed for listener-affecting changes.

#### Scenario: Invalid settings are rejected
- **WHEN** a client submits an invalid UI port, proxy port, secret suffix, route mode, or colliding UI/proxy ports
- **THEN** the API SHALL reject the request with a clear error and SHALL NOT persist the invalid value

#### Scenario: Listener setting changes require restart
- **WHEN** a valid UI port, proxy port, or secret path change is persisted
- **THEN** the API SHALL return `restart_needed: true` and either trigger the documented controlled restart behavior or clearly require an operator restart before claiming the new listener setting is active

#### Scenario: Route-only settings apply immediately
- **WHEN** a valid route mode or forced country setting changes without listener changes
- **THEN** the API SHALL persist the setting, mirror it into runtime state, and report that no restart is required

### Requirement: Management UI Workflow Parity
The Go management UI SHALL provide usable workflows for the core Python-era management actions.

#### Scenario: User can operate nodes from UI
- **WHEN** the user opens the authenticated management page
- **THEN** the UI SHALL show the node list, active node status, probe status, latency, route mode, and actions to refresh, test, connect, and disconnect nodes

#### Scenario: User can manage settings from UI
- **WHEN** the user opens settings controls
- **THEN** the UI SHALL allow credential updates, UI port, proxy port, secret suffix, route mode, and forced country changes with API validation feedback

#### Scenario: User can inspect gateway and logs
- **WHEN** the user requests gateway status or logs
- **THEN** the UI SHALL show service health, proxy egress result, OpenVPN state, worker health, and structured logs without requiring manual JSON inspection

### Requirement: Compatible State Log And Node Shapes
The Go runtime SHALL preserve Python-era JSON field compatibility for state, logs, and nodes while allowing additive Go fields.

#### Scenario: Nodes API includes settings state
- **WHEN** a client gets `/api/nodes`
- **THEN** the `state` payload SHALL include `username`, `port`, `secret_path`, `proxy_port`, `routing_mode`, `force_country`, active node identity, connection status, proxy health, and check/fetch status fields

#### Scenario: Node objects include display fields
- **WHEN** nodes are returned from `/api/nodes`, `/api/test_node`, or `/api/test_nodes`
- **THEN** each node SHALL include compatible fields for country, country short code, host name, IP, score, ping, speed, sessions, owner, ASN, AS name, location, IP type, quality, latency, proto, remote host/port, probe status, probe message, and probe timestamp where known

#### Scenario: Structured logs keep timestamp compatibility
- **WHEN** structured logs are written or returned through `/api/logs`
- **THEN** each log entry SHALL expose a `timestamp` field compatible with the Python log shape and MAY include additional Go-native time fields

#### Scenario: CLI logs read useful Go logs
- **WHEN** an operator runs the `ml logs` compatibility command
- **THEN** it SHALL show useful Go runtime logs rather than tailing an empty or stale file

### Requirement: CLI Config Preservation
The Go management CLI SHALL preserve existing and future UI auth configuration fields when updating individual settings.

#### Scenario: Port update preserves route settings
- **WHEN** `aimilivpnctl` or `ml` updates UI or proxy ports
- **THEN** existing `routing_mode`, `force_country`, `fixed_node_id`, username, password, secret path, and unknown JSON fields SHALL remain preserved unless explicitly changed

#### Scenario: Password update preserves listener settings
- **WHEN** `aimilivpnctl` or `ml` updates credentials
- **THEN** existing UI host, UI port, proxy port, route settings, secret path, and unknown JSON fields SHALL remain preserved unless explicitly changed
