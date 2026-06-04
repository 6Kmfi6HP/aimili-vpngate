## 1. Baseline And Regression Tests

- [x] 1.1 Add focused tests that demonstrate current Go gaps for OpenVPN command flags, TCP-only node validation, `/api/test_nodes`, `/api/test_proxy`, settings validation, state payload fields, log timestamp shape, and CLI config preservation.
- [x] 1.2 Add representative fixtures for Python-compatible nodes, state, UI auth config with routing fields, and structured log entries.
- [x] 1.3 Add a parity checklist in the change or migration docs that maps each accepted Python-era behavior to the Go package responsible for it.

## 2. Runtime Networking Parity

- [x] 2.1 Implement an OpenVPN command builder that adds explicit tun device, dev type, route-nopull behavior, IPv6 pull filters, retry/connect timeout flags, auth nocache, cipher compatibility flags, and TCP upstream proxy options.
- [x] 2.2 Update OpenVPN readiness parsing to produce user-facing status and diagnostic categories for auth, TLS, DNS, tun/tap, pushed option, timeout, and process startup failures.
- [x] 2.3 Replace TCP-only final candidate checks with bounded OpenVPN readiness probes using disposable tun devices, short timeouts, cleanup, and concurrency limits.
- [x] 2.4 Preserve TCP dialing as an optional prefilter while keeping UDP candidates eligible for OpenVPN readiness probing.
- [x] 2.5 Add Linux route setup and cleanup helpers for the service-owned route table/rules and loose `rp_filter`, with idempotent cleanup on disconnect, replacement, and shutdown.
- [x] 2.6 Implement real proxy egress health checks that verify listener reachability, `tun0` availability where applicable, outbound local-proxy connectivity, observed egress IP, latency, and failure reason.
- [x] 2.7 Add background proxy health monitoring that updates state and triggers auto, fixed-region, or fixed-IP recovery behavior according to route policy and invalid-node backoff.
- [x] 2.8 Add VPNGate API fetch fallback and diagnostics for configured upstream proxies, HTTPS fallback attempts, HTTP fallback where allowed, and non-mutating DNS/API failure categorization.

## 3. Management API State And CLI Parity

- [x] 3.1 Implement `/api/test_nodes` so it tests requested node IDs, updates persisted probe fields, and returns tested nodes.
- [x] 3.2 Update `/api/test_node`, `/api/test_proxy`, `/api/gateway_status`, `/api/check`, and `/api/refresh_nodes` to use the restored runtime checks and to write compatible state/log fields.
- [x] 3.3 Validate `/api/update_settings` and `/api/update_routing` inputs for port ranges, port collisions, secret suffix format, and allowed route modes before persistence.
- [x] 3.4 Return accurate `restart_needed` behavior for UI port, proxy port, and secret path changes, and document or implement controlled service restart behavior.
- [x] 3.5 Mirror UI config values into `/api/nodes` state output, including username, port, secret path, proxy port, route mode, forced country, active node, fetch/check state, and proxy health.
- [x] 3.6 Expand Go node JSON fields to preserve Python-compatible display and probe metadata while keeping Go-specific fields additive.
- [x] 3.7 Preserve structured log compatibility by exposing `timestamp` in log entries and making `ml logs` show useful Go runtime logs.
- [x] 3.8 Update `aimilivpnctl` config reads/writes so route settings, fixed node ID, listener settings, credentials, secret path, and unknown JSON fields are not lost during partial updates.

## 4. Management UI Workflow Parity

- [x] 4.1 Replace the JSON-console UI with a usable authenticated management UI for nodes, active connection status, route mode, settings, gateway status, and logs.
- [x] 4.2 Add node actions for refresh, batch test, single test, connect, and disconnect with clear success/error feedback.
- [x] 4.3 Add settings workflows for credentials, UI port, proxy port, secret suffix, route mode, and forced country using the validated API responses.
- [x] 4.4 Add gateway status and log views that do not require users to manually inspect raw JSON.

## 5. Documentation And Verification

- [x] 5.1 Update migration and smoke-test docs to describe actual restored Go parity behavior and explicitly list deferred non-goals such as host DNS mutation and full IP enrichment.
- [x] 5.2 Run `gofmt ./...`, `go test ./...`, `go vet ./...`, and `go build ./cmd/aimilivpn`.
- [x] 5.3 Run `bash -n install.sh` and any focused installer or CLI checks affected by config/log behavior.
- [x] 5.4 Run `openspec validate restore-go-runtime-parity`.
- [x] 5.5 Perform or document native Linux smoke checks for service startup, VPNGate fetch fallback, OpenVPN node probe, active connect/disconnect route cleanup, proxy egress, settings restart signaling, and management UI workflows.
