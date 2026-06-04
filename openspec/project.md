# AimiliVPN Project Context

## Purpose

AimiliVPN is a Python 3 VPNGate/OpenVPN proxy gateway for Linux VPS hosts. It discovers and tests public VPNGate nodes, manages an active OpenVPN connection, exposes a built-in management web UI, and provides a local dual HTTP/SOCKS5 proxy for other local processes.

## Runtime

- Primary runtime is Linux with Python 3 and the Python standard library.
- Deployment targets include Debian, Ubuntu, Alpine, CentOS/RHEL-family systems, Rocky, AlmaLinux, and Fedora.
- The service commonly needs root privileges for OpenVPN, tun/tap access, route changes, iptables, and interface binding.
- `install.sh` installs system packages, deploys the repo to `/opt/aimilivpn`, creates a service, and installs the `ml` management shortcut.

## Code Map

- `vpngate_manager.py`: main process, web UI, node lifecycle, OpenVPN process management, state files, auth/session helpers, and API endpoints.
- `proxy_server.py`: built-in HTTP and SOCKS5 proxy, tun-bound outbound connections, DNS-over-tun helper, and client relay logic.
- `vpn_utils.py`: VPNGate parsing, upstream proxy detection, route/interface helpers, latency checks, and networking utilities.
- `README.md`: bilingual user-facing install, usage, and troubleshooting documentation.

## Constraints

- Preserve the zero-dependency Python standard-library posture unless a change proposal explicitly accepts the tradeoff.
- Keep the local proxy default bound to `127.0.0.1` unless a spec covers public exposure risks and controls.
- Treat credentials, UI auth, secret paths, proxy exposure, tun/tap, DNS, route changes, iptables/firewall behavior, and system service changes as security-sensitive.
- Maintain compatibility with both systemd and OpenRC where installer/service behavior changes.
- Keep user-facing docs bilingual when changing documented behavior.

## Verification Expectations

- Run `python3 -m py_compile vpngate_manager.py proxy_server.py vpn_utils.py` for Python changes.
- For installer changes, use shell syntax checks such as `bash -n install.sh`.
- For OpenSpec changes, run `openspec list`, `openspec list --specs`, and `openspec validate <change-or-spec>` when applicable.
- For runtime-sensitive changes, include a manual smoke plan for service start, management UI, proxy connectivity, VPNGate fetch, and route/proxy behavior.
