# AimiliVPN Go Runtime Migration Checklist

This checklist captures the current Python runtime surface that the Go migration must preserve or intentionally replace.

## Runtime Entry Points

- Current manager: `vpngate_manager.py`
- Current proxy: `proxy_server.py`
- Current network helpers: `vpn_utils.py`
- Current installer and management shortcut: `install.sh` generates `/usr/bin/ml`
- Target runtime: one Go binary named `aimilivpn`

## Environment Variables

- `VPNGATE_DATA_DIR`: persistent data directory override
- `FETCH_INTERVAL_SECONDS`: VPNGate refresh interval, default `960`
- `CHECK_INTERVAL_SECONDS`: node check interval, default `960`
- `TARGET_VALID_NODES`: valid node target, default `3`
- `MAX_SCAN_ROWS`: maximum VPNGate rows to scan, default `300`
- `OPENVPN_TEST_TIMEOUT_SECONDS`: OpenVPN readiness timeout, default `35`
- `OPENVPN_CMD`: OpenVPN executable or command, default `openvpn`
- `OPENVPN_AUTH_USER`: OpenVPN username, default `vpn`
- `OPENVPN_AUTH_PASS`: OpenVPN password, default `vpn`
- `LOCAL_PROXY_HOST`: local proxy bind host, default `127.0.0.1`
- `LOCAL_PROXY_PORT`: local proxy port, default `7928`
- `UI_HOST`: management UI bind host, default `::`
- `UI_PORT`: management UI port, default `8787`
- `INVALID_BACKOFF_SECONDS`: invalid-node backoff, default `1800`

## Persistent Files

- `vpngate_data/ui_auth.json`: management UI auth, secret path, ports, and routing mode
- `vpngate_data/vpngate_auth.txt`: OpenVPN username/password file
- `vpngate_data/state.json`: runtime status used by UI and `ml status`
- `vpngate_data/nodes.json`: cached VPNGate nodes and probe results
- `vpngate_data/configs/*.ovpn`: generated OpenVPN node configs
- `vpngate_data/vpngate.log`: process log
- `vpngate_data/logs/YYYY-MM-DD.json`: structured log entries
- `vpngate_data/public_ip.txt`: last observed public IP where present

## Management UI And API Surface

- `GET /<secret>/` and `GET /<secret>/index.html`: management UI
- `GET /<secret>/api/nodes`: node list plus runtime state
- `GET /<secret>/configs/<file>`: generated OpenVPN profile download
- `GET /<secret>/api/gateway_status`: web, proxy, OpenVPN, and worker status
- `GET /<secret>/api/logs`: structured logs
- `POST /<secret>/api/login`: username/password login and session cookie
- `POST /<secret>/api/logout`: session removal
- `POST /<secret>/api/update_credentials`: username/password update
- `POST /<secret>/api/update_settings`: UI port, proxy port, secret suffix, and route settings
- `POST /<secret>/api/update_routing`: route mode update
- `POST /<secret>/api/check`: synchronous node maintenance request
- `POST /<secret>/api/refresh_nodes`: background node refresh
- `POST /<secret>/api/test_nodes`: test selected nodes
- `POST /<secret>/api/disconnect`: stop active OpenVPN process
- `POST /<secret>/api/connect`: connect selected node
- `POST /<secret>/api/test_node`: test one node
- `POST /<secret>/api/test_proxy`: local proxy egress check

## Route Modes

- `auto`: automatically maintain valid nodes and recover to another healthy node
- `fixed_region`: prefer nodes from a configured country or region
- `fixed_ip`: keep the selected node and avoid automatic movement to another IP

## Proxy Behavior

- One local listener supports HTTP proxy requests and SOCKS5 clients.
- Native default bind is `127.0.0.1:7928`.
- Public proxy exposure is opt-in through explicit configuration.
- Outbound connections should prefer tun-bound traffic on Linux when the VPN is active.
- DNS-over-tun fallback is used for hostnames where direct resolution would leak or fail.

## OpenVPN Behavior

- Generate per-node `.ovpn` files under the data directory.
- Write OpenVPN credentials to a private auth file.
- Launch the configured OpenVPN command as a subprocess.
- Detect readiness from OpenVPN output.
- Log stdout and stderr.
- Stop the active process on disconnect or replacement.
- Diagnose missing command, auth failures, DNS failures, TLS failures, tun/tap failures, route failures, and firewall obstruction where possible.

## Installer And Management Commands

- Native install path currently defaults to `/opt/aimilivpn`.
- Native service manager support includes systemd and OpenRC.
- Management shortcut command compatibility target: `ml`
- Lifecycle commands: `start`, `stop`, `restart`, `status`, `logs`, `update`, `uninstall`, `web`, `port`, `password`
- New installer must add explicit `native` and `docker` modes while preserving upgrade and data-safety behavior.

## Release And Deployment Targets

- Native Linux binary release is the primary supported runtime.
- Docker images must include the Go binary, OpenVPN, CA certificates, and Linux networking tools.
- Compose defaults must bind host ports to loopback and persist `/data`.
- GitHub Actions must validate Go, shell, Docker, and OpenSpec artifacts.
- Tagged releases must publish binary archives, checksums, and multi-platform Linux images.
