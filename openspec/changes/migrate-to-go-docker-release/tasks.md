## 1. Baseline And Project Structure

- [x] 1.1 Inventory current Python routes, API endpoints, environment variables, state files, auth files, log files, service behavior, and management commands into an implementation checklist.
- [x] 1.2 Add a Go module with `cmd/aimilivpn` and internal packages for config, state, VPNGate, OpenVPN, proxy, web/API, diagnostics, and service lifecycle.
- [x] 1.3 Add representative fixtures for VPNGate API rows, existing `vpngate_data` files, UI auth data, and OpenVPN configs.
- [x] 1.4 Add initial Go test scaffolding for parsing, config defaults, state persistence, and route/proxy diagnostics.

## 2. Go Runtime Core

- [x] 2.1 Implement Go configuration loading with existing environment variable names and defaults.
- [x] 2.2 Implement data directory creation, existing data preservation, and safe migration/backups for compatible `vpngate_data` files.
- [x] 2.3 Implement VPNGate fetch, CSV parsing, node normalization, candidate filtering, and concurrent latency/check workflows.
- [x] 2.4 Implement node cache and runtime state persistence compatible with current user-facing status behavior.
- [x] 2.5 Implement OpenVPN config generation and authentication file handling.
- [x] 2.6 Implement OpenVPN subprocess start, readiness detection, timeout handling, stdout/stderr logging, and controlled termination.
- [x] 2.7 Implement tun/tap, DNS, route, firewall, and missing-command diagnostics with clear user-facing errors.
- [x] 2.8 Implement auto mode, fixed country mode, fixed IP mode, backoff behavior, and recovery after unexpected OpenVPN exit.

## 3. Local Proxy

- [x] 3.1 Port the dual HTTP and SOCKS5 proxy behavior to Go with protocol tests for CONNECT, plain HTTP, and SOCKS5 flows.
- [x] 3.2 Implement tun-bound outbound connection behavior and DNS-over-tun fallback where supported on Linux.
- [x] 3.3 Preserve native default proxy binding to `127.0.0.1:7928`.
- [x] 3.4 Require explicit configuration and documentation for non-loopback proxy binding.
- [x] 3.5 Add proxy failure tests for missing tun device, failed DNS resolution, unreachable target, and client disconnects.

## 4. Management UI And API

- [x] 4.1 Port or embed the management UI assets in the Go binary.
- [x] 4.2 Implement management UI routing, secret path handling, authentication, sessions, and credential persistence.
- [x] 4.3 Implement API endpoints for node list, update/check actions, connection mode changes, active connection status, diagnostics, and logs.
- [x] 4.4 Preserve default management UI port `8787` and existing configurable UI host/port behavior.
- [x] 4.5 Add API and UI workflow tests for login, status fetch, node update, mode selection, logs, and error reporting.

## 5. Native Packaging

- [x] 5.1 Add systemd service templates for the Go binary with environment file support and restart behavior.
- [x] 5.2 Add OpenRC service templates for the Go binary with equivalent lifecycle behavior where practical.
- [x] 5.3 Add native install layout under `/opt/aimilivpn` or the documented replacement, including binary, data directory, config, and logs.
- [x] 5.4 Implement a management CLI or script that preserves `ml` compatibility for start, stop, restart, status, logs, update, web config, port config, password config, and uninstall.
- [x] 5.5 Add native service smoke instructions for service start, management UI reachability, proxy reachability, VPNGate fetch, and OpenVPN process handling.

## 6. Docker And Compose

- [x] 6.1 Add a multi-stage Dockerfile that builds the Go binary and includes OpenVPN, CA certificates, and required Linux networking tools in the runtime image.
- [x] 6.2 Add container startup checks for `/dev/net/tun`, required capabilities, OpenVPN availability, data directory writability, and port configuration.
- [x] 6.3 Add default Compose configuration with persistent data, required device/capability settings, restart policy, and host loopback port publishing.
- [x] 6.4 Add documented Compose override or example for explicit public exposure.
- [x] 6.5 Add documented fallback guidance for `network_mode: host` and privileged mode without making either the default.
- [x] 6.6 Verify `docker build` and `docker compose config` for the added files.

## 7. Installer Redesign

- [x] 7.1 Rewrite `install.sh` around explicit native and Docker modes with non-interactive flags and interactive prompts.
- [x] 7.2 Implement distro package detection and installation or guidance for OpenVPN, iproute/iptables tools, curl, tar, and Docker/Compose prerequisites.
- [x] 7.3 Implement native install and upgrade from GitHub Release binaries while preserving persistent data and credentials.
- [x] 7.4 Implement Docker install and upgrade using the published image and Compose files.
- [x] 7.5 Implement status, logs, start, stop, restart, update, configuration, and uninstall commands for both native and Docker deployment modes.
- [x] 7.6 Implement safe uninstall that separates service removal from data deletion and requires explicit confirmation before deleting data.
- [x] 7.7 Add `bash -n install.sh` verification and focused shell tests or dry-run coverage for mode parsing and lifecycle commands.

## 8. GitHub Actions And Release Workflow

- [x] 8.1 Add a pull request validation workflow for gofmt, go test, go vet, Go build, installer syntax, Docker build viability, and OpenSpec validation.
- [x] 8.2 Add a tagged release workflow for versioned Go binary archives across the supported OS/architecture matrix.
- [x] 8.3 Add checksum generation and upload for all downloadable release artifacts.
- [x] 8.4 Add Docker Buildx/QEMU release jobs for multi-platform Linux images, at minimum `linux/amd64` and `linux/arm64`.
- [x] 8.5 Publish container images to the configured registry with immutable version tags and documented moving tags.
- [x] 8.6 Declare least-privilege GitHub Actions permissions for validation, release asset upload, and container publishing.

## 9. Documentation

- [x] 9.1 Update the bilingual README for Go runtime behavior, native install, Docker install, Compose usage, lifecycle commands, ports, volumes, and security defaults.
- [x] 9.2 Document migration from the Python release, including data backup, rollback, and compatibility notes.
- [x] 9.3 Document release assets, supported OS/architecture matrix, image tags, checksum verification, and Linux-only VPN runtime limitations.
- [x] 9.4 Document manual smoke tests for native and Docker deployments.

## 10. Verification

- [x] 10.1 Run `go fmt ./...`, `go test ./...`, `go vet ./...`, and `go build ./cmd/aimilivpn`.
- [x] 10.2 Run `bash -n install.sh`.
- [x] 10.3 Run Docker build and Compose configuration validation.
- [x] 10.4 Run OpenSpec validation for `migrate-to-go-docker-release`.
- [x] 10.5 Perform a native Linux smoke test for service start, management UI, VPNGate fetch, OpenVPN connection handling, and local proxy connectivity.
- [x] 10.6 Perform a Docker smoke test for tun/capability diagnostics, Compose startup, management UI, persistent data, and local proxy connectivity.
