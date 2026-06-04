# Native Go Packaging

## Layout

- `/opt/aimilivpn/bin/aimilivpn`: Go service binary
- `/opt/aimilivpn/bin/aimilivpnctl`: lifecycle helper
- `/usr/bin/ml`: compatibility symlink or wrapper to `aimilivpnctl`
- `/etc/default/aimilivpn`: optional environment overrides
- `/opt/aimilivpn/vpngate_data`: persistent data, auth, logs, state, node cache, and generated OpenVPN configs
- systemd unit: `/etc/systemd/system/aimilivpn.service`
- OpenRC script: `/etc/init.d/aimilivpn`

## Native Smoke Plan

1. Run `aimilivpn check` and confirm required commands, data directory, and tun access are reported clearly.
2. Start the service with `systemctl start aimilivpn` or `rc-service aimilivpn start`.
3. Run `ml status` and confirm the management URL, proxy port, data directory, and active state are printed.
4. Open the management UI through the printed secret URL.
5. Trigger node refresh from the UI or `POST /api/refresh_nodes`.
6. Connect a node and confirm OpenVPN process state and local proxy reachability.
7. Run `ml logs` and confirm service logs are available.

## Rollback

The native installer must preserve `/opt/aimilivpn/vpngate_data` unless the user explicitly confirms data deletion. To roll back to a Python release, stop the Go service, restore the previous release files, keep the data directory backup, and restart the previous service.
