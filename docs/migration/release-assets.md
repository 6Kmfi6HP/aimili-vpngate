# Release Assets

Tagged releases publish:

- `aimilivpn_<version>_<os>_<arch>.tar.gz`: versioned binary archive
- `aimilivpn_<os>_<arch>.tar.gz`: moving binary archive name used by latest installer downloads
- `checksums.txt`: SHA-256 checksums for downloadable archives
- GHCR image tags for the release version, minor version, and `latest`

## Supported Binary Matrix

- `linux/amd64`
- `linux/arm64`
- `linux/arm/v7`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`
- `windows/arm64`

Full VPN runtime behavior is supported only on Linux hosts with OpenVPN, tun/tap, routing, and firewall privileges. macOS and Windows binaries are built for release completeness and auxiliary command compatibility.

## Supported Container Platforms

- `linux/amd64`
- `linux/arm64`

## Checksum Verification

```bash
sha256sum -c checksums.txt
```

Verify downloaded archives before installing them on production VPS hosts.
