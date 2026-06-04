## Context

AimiliVPN currently runs as Python source on Linux VPS hosts. `vpngate_manager.py` owns the manager process, management web UI, API endpoints, state files, OpenVPN subprocess lifecycle, and background node lifecycle. `proxy_server.py` provides the local HTTP/SOCKS5 proxy and tun-bound outbound sockets. `vpn_utils.py` handles VPNGate parsing, upstream proxy detection, route/interface helpers, latency checks, and OpenVPN/network diagnostics. `install.sh` deploys source to `/opt/aimilivpn`, writes systemd or OpenRC service files, initializes `vpngate_data`, and installs the `ml` management shortcut.

The migration intentionally changes the default runtime from Python standard library scripts to a Go binary. The project should keep the current operational shape: Linux-first, OpenVPN-based, root or elevated networking privileges, local-only proxy by default, management UI on port 8787, proxy on port 7928, and compatibility with existing service-oriented VPS installs.

## Goals / Non-Goals

**Goals:**

- Replace the Python runtime with a Go service while preserving existing user-facing VPNGate, OpenVPN, proxy, management UI, mode selection, state, and logging behavior.
- Produce Docker images and Compose examples that make the required Linux networking privileges explicit.
- Provide GitHub Actions workflows for validation, tagged releases, cross-OS binary assets, checksums, and multi-platform Linux container images.
- Redesign installation around clear native and Docker modes with idempotent lifecycle commands.
- Keep dependency growth controlled by using Go standard library packages for the web server, proxy, subprocess management, JSON state, and embedding wherever practical.

**Non-Goals:**

- Do not change the upstream VPNGate protocol.
- Do not replace OpenVPN with another tunnel implementation.
- Do not make the proxy or management UI public by default.
- Do not introduce Kubernetes support.
- Do not guarantee complete VPN routing behavior outside Linux, even if non-Linux helper binaries are published.

## Decisions

### Decision: Use One Go Service Binary

Build a single `aimilivpn` Go binary with internal packages for VPNGate fetching/parsing, OpenVPN process control, proxy serving, route/network diagnostics, state/config, and web/API handling. The binary should serve the management UI with `net/http` and embedded static assets or templates.

Rationale: the current project is operationally one service. A single binary keeps install, service management, Docker image construction, and release assets simple.

Alternatives considered:

- Multiple daemons for manager and proxy: rejected for this migration because it increases service orchestration, health checks, and upgrade ordering.
- A Go web framework: rejected unless implementation proves a strong need; `net/http` is enough for the current UI/API surface and keeps dependencies small.

### Decision: Preserve Data Directory and Environment Names

Use a data directory equivalent to `vpngate_data`, defaulting to a service-owned directory in native installs and `/data` in containers. Preserve important environment names and defaults such as `VPNGATE_DATA_DIR`, `FETCH_INTERVAL_SECONDS`, `CHECK_INTERVAL_SECONDS`, `TARGET_VALID_NODES`, `MAX_SCAN_ROWS`, `OPENVPN_TEST_TIMEOUT_SECONDS`, `OPENVPN_CMD`, `OPENVPN_AUTH_USER`, `OPENVPN_AUTH_PASS`, `LOCAL_PROXY_HOST`, `LOCAL_PROXY_PORT`, `UI_HOST`, and `UI_PORT`.

Rationale: stable configuration names reduce migration risk for existing users and make Docker env configuration predictable.

Alternatives considered:

- Rename all configuration into a new YAML file only: rejected because it would make upgrades more brittle. A config file can be added later, but environment compatibility should remain in this change.

### Decision: Keep OpenVPN External to the Go Process

The Go service should continue launching an OpenVPN process using generated node configs rather than embedding a VPN protocol implementation. Native installs install or verify `openvpn`; Docker images include the OpenVPN package and required Linux networking tools.

Rationale: this preserves battle-tested tunnel behavior and limits the rewrite to orchestration, proxying, and UI behavior.

Alternatives considered:

- Reimplement OpenVPN behavior in Go: rejected as too large and security-sensitive for this migration.
- Vendor a custom OpenVPN binary: rejected because distro packages and container packages are easier to patch and audit.

### Decision: Treat Container Network Exposure as Host-Binding Policy

Docker images need the process to listen on container interfaces when ports are published, but Compose defaults must publish management and proxy ports to `127.0.0.1` on the host unless the user explicitly opts into public binding. Compose should mount `/dev/net/tun`, add only required capabilities, persist `/data`, and document when `network_mode: host` is appropriate.

Rationale: binding to `127.0.0.1` inside a container can make published ports unreachable from the host. Local-only exposure must therefore be enforced at the Docker host port binding layer for container deployments.

Alternatives considered:

- Run the container with host networking by default: rejected because it is less explicit about exposure and can collide with host ports.
- Require privileged containers for all deployments: rejected; use targeted device and capability configuration first, with privileged mode only as a documented fallback.

### Decision: Split Packaging Assets from Installer Logic

Keep install templates and packaging assets in versioned files, such as Dockerfile, Compose examples, systemd/OpenRC templates, and release workflow files. The installer should consume these assets or release artifacts rather than embedding large generated scripts inside heredocs.

Rationale: the current installer is difficult to review because it embeds substantial management code. Splitting assets makes review, testing, and release validation cleaner.

Alternatives considered:

- Keep a monolithic installer script: rejected because it repeats the current maintainability problem.
- Replace the installer entirely with Compose: rejected because native VPS installs remain a supported user path.

### Decision: Publish Linux Images and Cross-OS Binaries

Release automation should publish Docker images for Linux platforms supported by Docker Buildx, at minimum `linux/amd64` and `linux/arm64`, with `linux/arm/v7` when the runtime image and OpenVPN packages support it. Binary assets should include Linux primary targets and auxiliary macOS/Windows builds where the code compiles, with documentation that full VPN runtime support is Linux-only.

Rationale: Docker images are Linux artifacts, while Go cross-compilation can still produce useful binaries for inspection, CLI subcommands, or future non-VPN helpers.

Alternatives considered:

- Only ship Docker images: rejected because native systemd/OpenRC installs remain part of the project.
- Only ship Linux binaries: rejected because the user explicitly asked for different-system build coverage.

## Risks / Trade-offs

- [Behavior drift during rewrite] -> Build parity tasks around current API routes, state files, proxy behavior, OpenVPN lifecycle, and UI workflows before removing Python entrypoints.
- [Container networking confusion] -> Provide a secure default Compose file, comments for `127.0.0.1` host publishing, and a separate opt-in public exposure example.
- [Missing tun/capability privileges] -> Add startup diagnostics that fail fast with actionable messages when `/dev/net/tun`, `CAP_NET_ADMIN`, routing tools, or OpenVPN are missing.
- [Over-broad container privileges] -> Prefer `/dev/net/tun` plus targeted capabilities and document privileged mode only as a fallback.
- [Release workflow supply-chain risk] -> Pin workflow actions by major version or commit, generate checksums, use GitHub token permissions narrowly, and publish images only from trusted tags.
- [Installer destructive upgrades] -> Preserve `vpngate_data`, back up changed config/state before migration, and require confirmation before uninstalling data.
- [Non-Linux binaries imply unsupported behavior] -> Label release assets and docs clearly: full VPN runtime support requires Linux networking privileges.

## Migration Plan

1. Add the Go module, package structure, and parity tests while leaving Python files in place.
2. Port VPNGate parsing, node scoring, state/config handling, and OpenVPN config generation.
3. Port OpenVPN process lifecycle, diagnostics, local proxy behavior, and management API/web UI.
4. Add native service packaging and update installer support for the Go binary.
5. Add Dockerfile and Compose examples with secure defaults and runtime diagnostics.
6. Add GitHub Actions validation and release workflows.
7. Update bilingual README and migration docs.
8. Mark Python runtime as legacy or remove it only after the Go service passes parity checks and release packaging is complete.

Rollback strategy: keep the Python release branch or previous tagged release installable until at least one Go release is verified. Native installs should back up existing data before migration, and Docker installs should store state in named volumes or bind mounts so the container can be replaced safely.

## Open Questions

- Which image registry names should be official: GitHub Container Registry only, Docker Hub, or both?
- Should the first Go release keep the Python files as a legacy fallback in the repository, or remove them from the release artifact once parity is complete?
- Which Linux architectures beyond `amd64` and `arm64` are required for the first container release?
- Should `ml` remain the management command name, or should a new `aimilivpnctl` command be introduced with `ml` as a compatibility alias?
