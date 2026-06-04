#!/usr/bin/env sh
set -eu

mkdir -p "${VPNGATE_DATA_DIR:-/data}"

echo "[AimiliVPN] Running container diagnostics..."
aimilivpn check || true

if [ ! -e /dev/net/tun ]; then
  echo "[AimiliVPN] WARNING: /dev/net/tun is missing. Add devices: ['/dev/net/tun:/dev/net/tun'] or equivalent tun access." >&2
fi

if ! command -v openvpn >/dev/null 2>&1; then
  echo "[AimiliVPN] ERROR: openvpn is not available in the image." >&2
  exit 1
fi

if [ ! -w "${VPNGATE_DATA_DIR:-/data}" ]; then
  echo "[AimiliVPN] ERROR: data directory ${VPNGATE_DATA_DIR:-/data} is not writable." >&2
  exit 1
fi

exec aimilivpn "$@"
