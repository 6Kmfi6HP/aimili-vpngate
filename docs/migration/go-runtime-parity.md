# Go Runtime Parity Checklist

This checklist maps accepted Python-era behavior to the Go package that owns it for `restore-go-runtime-parity`.

| Python-era behavior | Go owner | Verification |
| --- | --- | --- |
| OpenVPN command flags, auth file, route-nopull, cipher compatibility, upstream proxy options | `internal/openvpn` | Unit tests for `BuildCommand`; fake OpenVPN readiness tests |
| OpenVPN readiness diagnostics for auth, TLS, DNS, tun/tap, pushed options, and timeouts | `internal/openvpn`, `internal/app` | Fake OpenVPN output tests; state/log checks |
| Candidate validation through bounded OpenVPN probes, with TCP as prefilter only | `internal/vpngate`, `internal/app` | Probe callback tests; native smoke test |
| Linux policy route setup, loose `rp_filter`, and cleanup | `internal/app` | Command-helper tests where practical; native smoke test |
| Local HTTP/SOCKS5 proxy listener and tun-bound egress | `internal/proxy`, `internal/app` | Proxy protocol tests; `/api/test_proxy` smoke test |
| Proxy egress health, state fields, and auto/fixed recovery behavior | `internal/app`, `internal/web` | API tests; native smoke test |
| VPNGate API fetch fallback and non-mutating diagnostics | `internal/vpngate` | Fetch fallback tests; smoke test with blocked/failing endpoint |
| Management API routes and settings validation | `internal/web`, `internal/app` | API route tests |
| Compatible state, log, and node JSON shape | `internal/state`, `internal/vpngate`, `internal/app` | Fixture compatibility tests |
| CLI config/log compatibility for `ml` workflows | `cmd/aimilivpnctl` | CLI package tests |
| Management UI node/settings/status/log workflows | `internal/web` | API-backed UI smoke test |

Listener-affecting setting changes are intentionally restart-signaled rather than hot-rebound. When the UI port, proxy port, or secret path changes, the API returns `restart_needed: true`; operators should run `ml restart` or restart the service manager before expecting the new listener setting to be active.

Deferred parity items:

- Do not mutate `/etc/resolv.conf`.
- Do not require full third-party IP metadata enrichment before core node actions work.
- Do not make proxy or management UI public by default.
- Do not guarantee full VPN runtime behavior outside Linux hosts with OpenVPN and required networking privileges.
