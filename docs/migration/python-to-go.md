# Python To Go Migration

## Before Migrating

1. Stop the current service.
2. Back up the existing data directory, usually `/opt/aimilivpn/vpngate_data`.
3. Record the current management URL, secret suffix, proxy port, and route mode.
4. Keep the previous Python release or branch available for rollback.

## Data Compatibility

The Go runtime reads the existing `ui_auth.json`, `state.json`, `nodes.json`, `vpngate_auth.txt`, logs, and generated configs where formats remain compatible. On first Go runtime startup, the data store creates a one-time backup marker under `vpngate_data/backups/go-migration-*`.

## Native Migration

```bash
sudo bash install.sh upgrade --mode native
sudo ml status
sudo ml logs
```

The native path installs Go binaries under `/opt/aimilivpn/bin`, keeps `/usr/bin/ml` as the management shortcut, and preserves persistent data unless uninstall is run with `--delete-data`.

## Docker Migration

```bash
sudo bash install.sh install --mode docker
docker compose -f /opt/aimilivpn/docker-compose.yml logs -f aimilivpn
```

For Docker, copy or mount the old data directory into the configured `/data` volume or bind mount before starting the container.

## Rollback

1. Stop the Go service or container.
2. Restore the previous Python release files.
3. Restore the data directory from the pre-migration backup if needed.
4. Restart the previous service manager entry.

The Go migration does not require changing VPNGate accounts or upstream protocol settings.
