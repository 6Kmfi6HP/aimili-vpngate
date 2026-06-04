## Why

AimiliVPN is currently delivered as a large Python script stack plus a host-level installer, which makes reproducible builds, upgrades, release validation, and container deployment harder than they need to be. Rewriting the runtime in Go and formalizing Docker, Compose, release automation, and installation flows will make deployment more predictable while keeping the existing VPNGate/OpenVPN gateway behavior recognizable for current users.

## What Changes

- **BREAKING**: Replace the Python runtime entrypoints with a Go implementation as the primary supported runtime.
- Preserve the current product behavior: VPNGate node discovery and benchmarking, OpenVPN lifecycle management, management web UI, auto/fixed route modes, local HTTP/SOCKS5 proxy, state/log visibility, and default local-only proxy binding.
- Add Docker image support for the Go service, including documented requirements for `/dev/net/tun`, Linux capabilities, host networking or explicit port mapping, persistent volumes, and OpenVPN/system networking tools.
- Add Docker Compose examples for common deployments, including secure local-only defaults and opt-in public exposure controls.
- Add GitHub Actions workflows for pull request validation and tagged releases.
- Add release automation that builds Go binaries for supported operating systems/architectures and publishes multi-platform container images.
- Redesign the installer into a clear native/Docker installation interface with install, upgrade, status, logs, restart, and uninstall paths.
- Update bilingual documentation for native install, Docker install, Compose, releases, upgrades, troubleshooting, and migration from the Python release.

## Capabilities

### New Capabilities

- `go-runtime`: Defines the Go service behavior and compatibility expectations for replacing the current Python manager, proxy, VPNGate utilities, and web UI/API surface.
- `container-deployment`: Defines Docker image and Docker Compose deployment behavior, required host privileges, configuration, volumes, ports, and safe network exposure defaults.
- `release-automation`: Defines CI and release workflows for validation, multi-OS Go binaries, multi-platform images, checksums, and release publishing.
- `installer-redesign`: Defines the redesigned installation and lifecycle-management script behavior for native and Docker-based deployments.

### Modified Capabilities

- None. No existing OpenSpec specs are present; this change introduces the first formal capability specs for the migration.

## Impact

- User impact: users get a clearer choice between native and Docker deployment, predictable upgrades, tagged releases, downloadable binaries, and Compose-based operations.
- Operational impact: deployments will depend on Go-built artifacts and container images instead of running Python source directly; Docker deployments will need explicit tun/tap, capability, networking, and volume configuration.
- Security impact: proxy exposure remains local-only by default; any public proxy/UI binding must stay explicit and documented because credentials, routes, DNS, tun/tap, firewall behavior, and OpenVPN process control are security-sensitive.
- Compatibility impact: Linux remains the primary runtime target; native service management must continue supporting systemd and OpenRC where practical. Non-Linux release artifacts may be built for tooling compatibility, but full VPN runtime behavior is only guaranteed on Linux hosts with the required networking privileges.
- Documentation impact: README and install docs must become bilingual for Go, Docker, Compose, GitHub release assets, upgrade, uninstall, and migration paths.

### Non-Goals

- Do not change the VPNGate protocol or require a non-VPNGate upstream service.
- Do not make the local proxy public by default.
- Do not replace OpenVPN with WireGuard or another tunnel protocol in this change.
- Do not require Kubernetes or another orchestrator beyond Docker Compose examples.
- Do not guarantee full VPN routing behavior on macOS or Windows hosts; cross-platform binaries are for release completeness and auxiliary use unless later specs expand support.
