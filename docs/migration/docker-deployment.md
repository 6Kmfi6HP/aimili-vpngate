# Docker And Compose Deployment

## Secure Default

The default `docker-compose.yml` publishes the management UI and local proxy only on the Docker host loopback address:

- `127.0.0.1:8787:8787`
- `127.0.0.1:7928:7928`

Inside the container the service listens on `0.0.0.0` so Docker port publishing can work. The host binding keeps the public exposure local-only by default.

## Required Linux Runtime Access

The container requires:

- `/dev/net/tun:/dev/net/tun`
- `NET_ADMIN`
- `NET_RAW`
- writable persistent `/data`
- OpenVPN and Linux networking tools in the image

The default Compose file uses targeted device and capability configuration instead of `privileged: true`.

## Public Exposure Override

Use `docker-compose.public.yml` only when the management UI and proxy must be reachable outside the Docker host:

```bash
docker compose -f docker-compose.yml -f docker-compose.public.yml up -d
```

Before enabling public exposure, configure strong UI credentials and host firewall rules. The proxy can be abused if exposed without controls.

## Fallbacks

Some VPS/container hosts restrict tun/tap or policy routing in ways targeted capabilities cannot solve. If diagnostics show tun, route, or capability failures, operators can evaluate these fallbacks manually:

- `network_mode: host`: avoids Docker port publishing and uses host networking directly.
- `privileged: true`: broadens container privileges and should only be used after targeted capabilities fail.

Neither fallback is the default because both increase blast radius.

## Compose Smoke Plan

1. Run `docker compose config`.
2. Run `docker compose up -d --build`.
3. Run `docker compose logs -f aimilivpn` and confirm diagnostics mention tun/OpenVPN/data directory state.
4. Open `http://127.0.0.1:8787/<secret>/` from the Docker host or through an SSH tunnel.
5. Check `http://127.0.0.1:7928` as the local proxy from the Docker host.
6. Recreate the container and verify `/data` preserves UI auth, state, nodes, configs, and logs.
