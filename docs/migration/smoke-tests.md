# AimiliVPN Smoke Tests

## Native Linux Smoke Test

1. `aimilivpn check`
2. `systemctl start aimilivpn` or `rc-service aimilivpn start`
3. `ml status`
4. Open the printed management URL.
5. Trigger node refresh from the UI.
6. Connect one node and confirm OpenVPN reports readiness.
7. Configure a local client to use `127.0.0.1:7928`.
8. Confirm logs are visible with `ml logs`.
9. Restart the service and confirm data persists.

## Go Runtime Parity Smoke Test

Run these checks on a Linux host with OpenVPN, `/dev/net/tun`, and route-management privileges. They document the native checks required by `restore-go-runtime-parity`; they are not expected to pass in an unprivileged build container.

1. Start the service and open the authenticated management URL.
2. Trigger **Refresh** and confirm `/api/refresh_nodes` writes fresh nodes, configs, fetch status, and logs.
3. Temporarily set `OPENVPN_UPSTREAM_HTTP` or `OPENVPN_UPSTREAM_SOCKS`, block direct access to `www.vpngate.net` if practical, and confirm the fetch diagnostic logs show the upstream proxy attempt before HTTPS/insecure/HTTP fallback.
4. Select one UDP and one TCP node, run **Test selected**, and confirm `/api/test_nodes` updates `probe_status`, `probe_message`, and `probed_at`; TCP reachability alone must not mark a node available when the OpenVPN probe fails.
5. Connect an available node and confirm OpenVPN readiness, the active node ID in `/api/nodes`, service-owned route table/rule setup, and loose `rp_filter` diagnostics where supported.
6. Configure a local client to use the local proxy, then run **Test proxy** and confirm `proxy_ok`, `proxy_ip`, `proxy_latency_ms`, and `proxy_error` are updated from real proxy egress.
7. Disconnect or replace the active node and confirm service-owned route rules/table entries are cleaned up without removing unrelated host routes.
8. Change only route mode or forced country and confirm the API reports no restart requirement and mirrors the setting in `/api/nodes`.
9. Change the UI port, proxy port, or secret path and confirm the API reports `restart_needed: true`; restart the service manually and verify the new listener setting is active.
10. Use the UI to refresh nodes, test one node, batch-test selected nodes, connect, disconnect, inspect gateway status, and inspect logs without manually reading raw JSON files.

## Docker Smoke Test

1. `docker compose config`
2. `docker compose up -d --build`
3. `docker compose logs --tail=120 aimilivpn`
4. Confirm diagnostics mention OpenVPN, tun access, and `/data` writability.
5. Open `http://127.0.0.1:8787/<secret>/` on the Docker host or through an SSH tunnel.
6. Confirm the proxy is published only on `127.0.0.1:7928` unless the public override is explicitly used.
7. Recreate the container and confirm `/data` preserves auth, state, nodes, configs, and logs.

## Security Checks

- Keep native proxy binding at `127.0.0.1` unless public exposure is intentional.
- Keep Docker host port publishing on `127.0.0.1` by default.
- Use strong management UI credentials before any public exposure.
- Prefer `/dev/net/tun` plus `NET_ADMIN`/`NET_RAW` over `privileged: true`.
- This Go parity change does not mutate `/etc/resolv.conf`; fix host DNS separately if the host resolver is broken.
- Full third-party IP metadata enrichment remains deferred; core node actions must work without it.
