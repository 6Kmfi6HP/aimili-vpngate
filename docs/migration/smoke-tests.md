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
