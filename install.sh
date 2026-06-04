#!/usr/bin/env bash
set -euo pipefail

REPO="${AIMILIVPN_REPO:-6Kmfi6HP/aimili-vpngate}"
INSTALL_DIR="${INSTALL_DIR:-/opt/aimilivpn}"
DATA_DIR="${VPNGATE_DATA_DIR:-${INSTALL_DIR}/vpngate_data}"
MODE="native"
ACTION="install"
VERSION="latest"
ASSUME_YES="false"
DRY_RUN="false"
DELETE_DATA="false"
PUBLIC_EXPOSURE="false"

usage() {
  cat <<'USAGE'
AimiliVPN installer

Usage:
  install.sh [action] [options]

Actions:
  install       Install AimiliVPN (default)
  upgrade       Upgrade installed assets
  status        Show service/container status
  logs          Show service/container logs
  start         Start service/container
  stop          Stop service/container
  restart       Restart service/container
  uninstall     Remove service/container assets

Options:
  --mode native|docker       Deployment mode (default: native)
  --version VERSION          GitHub release version or latest
  --install-dir DIR          Install directory (default: /opt/aimilivpn)
  --data-dir DIR             Persistent data directory
  --public                   Docker mode: write public exposure override
  --delete-data              Uninstall: delete persistent data after confirmation
  -y, --yes                  Non-interactive confirmation
  --dry-run                  Print commands without executing changes
  -h, --help                 Show help
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    install|upgrade|status|logs|start|stop|restart|uninstall)
      ACTION="$1"
      shift
      ;;
    --mode)
      MODE="${2:-}"
      shift 2
      ;;
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="${2:-}"
      DATA_DIR="${VPNGATE_DATA_DIR:-${INSTALL_DIR}/vpngate_data}"
      shift 2
      ;;
    --data-dir)
      DATA_DIR="${2:-}"
      shift 2
      ;;
    --public)
      PUBLIC_EXPOSURE="true"
      shift
      ;;
    --delete-data)
      DELETE_DATA="true"
      shift
      ;;
    -y|--yes)
      ASSUME_YES="true"
      shift
      ;;
    --dry-run)
      DRY_RUN="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [ "$MODE" != "native" ] && [ "$MODE" != "docker" ]; then
  echo "--mode must be native or docker" >&2
  exit 2
fi

run() {
  if [ "$DRY_RUN" = "true" ]; then
    printf '[dry-run]'
    printf ' %q' "$@"
    printf '\n'
    return 0
  fi
  "$@"
}

need_root() {
  if [ "$(id -u)" -ne 0 ] && [ "$DRY_RUN" != "true" ]; then
    echo "Please run as root." >&2
    exit 1
  fi
}

confirm() {
  prompt="$1"
  if [ "$ASSUME_YES" = "true" ]; then
    return 0
  fi
  printf '%s [y/N]: ' "$prompt"
  read -r answer
  case "$answer" in
    y|Y|yes|YES) return 0 ;;
    *) return 1 ;;
  esac
}

pkg_install() {
  if command -v apt-get >/dev/null 2>&1; then
    run apt-get update
    run apt-get install -y "$@"
  elif command -v dnf >/dev/null 2>&1; then
    run dnf install -y "$@"
  elif command -v yum >/dev/null 2>&1; then
    run yum install -y "$@"
  elif command -v apk >/dev/null 2>&1; then
    run apk add --no-cache "$@"
  else
    echo "No supported package manager found. Install manually: $*" >&2
  fi
}

install_native_packages() {
  pkg_install ca-certificates curl tar openvpn iproute2 iptables
}

arch_name() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    armv7l) echo "armv7" ;;
    *) uname -m ;;
  esac
}

asset_base() {
  arch="$(arch_name)"
  if [ "$VERSION" = "latest" ]; then
    echo "https://github.com/${REPO}/releases/latest/download/aimilivpn_linux_${arch}.tar.gz"
  else
    echo "https://github.com/${REPO}/releases/download/${VERSION}/aimilivpn_${VERSION}_linux_${arch}.tar.gz"
  fi
}

build_or_download_native() {
  run mkdir -p "${INSTALL_DIR}/bin" "$DATA_DIR"
  if [ -f "go.mod" ] && command -v go >/dev/null 2>&1; then
    run go build -trimpath -o "${INSTALL_DIR}/bin/aimilivpn" ./cmd/aimilivpn
    run go build -trimpath -o "${INSTALL_DIR}/bin/aimilivpnctl" ./cmd/aimilivpnctl
  else
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT
    url="$(asset_base)"
    run curl -fsSL "$url" -o "${tmp}/aimilivpn.tar.gz"
    run tar -xzf "${tmp}/aimilivpn.tar.gz" -C "$tmp"
    run install -m 0755 "${tmp}/aimilivpn" "${INSTALL_DIR}/bin/aimilivpn"
    run install -m 0755 "${tmp}/aimilivpnctl" "${INSTALL_DIR}/bin/aimilivpnctl"
  fi
  run ln -sf "${INSTALL_DIR}/bin/aimilivpnctl" /usr/bin/ml
}

write_native_config() {
  run mkdir -p "$DATA_DIR" /etc/default
  if [ ! -f /etc/default/aimilivpn ] || [ "$ACTION" = "install" ]; then
    if [ "$DRY_RUN" = "true" ]; then
      echo "[dry-run] write /etc/default/aimilivpn"
    else
      cat > /etc/default/aimilivpn <<EOF
VPNGATE_DATA_DIR=${DATA_DIR}
LOCAL_PROXY_HOST=127.0.0.1
LOCAL_PROXY_PORT=7928
UI_HOST=::
UI_PORT=8787
OPENVPN_CMD=openvpn
EOF
    fi
  fi
}

install_service() {
  if command -v systemctl >/dev/null 2>&1; then
    if [ "$DRY_RUN" = "true" ]; then
      echo "[dry-run] install systemd unit"
    else
      if [ -f packaging/systemd/aimilivpn.service ]; then
        install -m 0644 packaging/systemd/aimilivpn.service /etc/systemd/system/aimilivpn.service
      else
        cat > /etc/systemd/system/aimilivpn.service <<EOF
[Unit]
Description=AimiliVPN Go VPNGate/OpenVPN Manager
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=-/etc/default/aimilivpn
ExecStart=${INSTALL_DIR}/bin/aimilivpn serve
Restart=always
RestartSec=5
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW

[Install]
WantedBy=multi-user.target
EOF
      fi
    fi
    run systemctl daemon-reload
    run systemctl enable aimilivpn.service
  elif command -v rc-service >/dev/null 2>&1; then
    if [ "$DRY_RUN" = "true" ]; then
      echo "[dry-run] install OpenRC service"
    else
      if [ -f packaging/openrc/aimilivpn ]; then
        install -m 0755 packaging/openrc/aimilivpn /etc/init.d/aimilivpn
      else
        echo "OpenRC template missing; install from a full release archive." >&2
      fi
    fi
    run rc-update add aimilivpn default
  else
    echo "No supported service manager detected. Use ${INSTALL_DIR}/bin/aimilivpn serve manually." >&2
  fi
}

native_install() {
  need_root
  install_native_packages
  build_or_download_native
  write_native_config
  install_service
  service_action restart
  native_status
}

service_action() {
  action="$1"
  if command -v systemctl >/dev/null 2>&1; then
    run systemctl "$action" aimilivpn.service
  elif command -v rc-service >/dev/null 2>&1; then
    run rc-service aimilivpn "$action"
  else
    echo "No supported service manager found." >&2
    return 1
  fi
}

native_status() {
  if [ -x "${INSTALL_DIR}/bin/aimilivpnctl" ]; then
    run "${INSTALL_DIR}/bin/aimilivpnctl" status
  elif command -v ml >/dev/null 2>&1; then
    run ml status
  else
    service_action status || true
  fi
}

native_logs() {
  if [ -x "${INSTALL_DIR}/bin/aimilivpnctl" ]; then
    run "${INSTALL_DIR}/bin/aimilivpnctl" logs
  elif [ -f "${DATA_DIR}/vpngate.log" ]; then
    run tail -n 120 "${DATA_DIR}/vpngate.log"
  elif command -v journalctl >/dev/null 2>&1; then
    run journalctl -u aimilivpn.service -n 120 --no-pager
  fi
}

write_compose_files() {
  run mkdir -p "$INSTALL_DIR" "$DATA_DIR"
  if [ "$DRY_RUN" = "true" ]; then
    echo "[dry-run] write Docker Compose files to ${INSTALL_DIR}"
    return 0
  fi
  if [ -f docker-compose.yml ]; then
    cp docker-compose.yml "${INSTALL_DIR}/docker-compose.yml"
  else
    cat > "${INSTALL_DIR}/docker-compose.yml" <<'EOF'
services:
  aimilivpn:
    image: ghcr.io/6kmfi6hp/aimili-vpngate:latest
    restart: unless-stopped
    environment:
      AIMILIVPN_CONTAINER: "true"
      VPNGATE_DATA_DIR: /data
      UI_HOST: 0.0.0.0
      LOCAL_PROXY_HOST: 0.0.0.0
    ports:
      - "127.0.0.1:8787:8787"
      - "127.0.0.1:7928:7928"
    cap_add:
      - NET_ADMIN
      - NET_RAW
    devices:
      - /dev/net/tun:/dev/net/tun
    volumes:
      - aimilivpn-data:/data
volumes:
  aimilivpn-data:
EOF
  fi
  if [ "$PUBLIC_EXPOSURE" = "true" ]; then
    if [ -f docker-compose.public.yml ]; then
      cp docker-compose.public.yml "${INSTALL_DIR}/docker-compose.public.yml"
    fi
  fi
}

compose() {
  if ! command -v docker >/dev/null 2>&1; then
    echo "Docker is not installed. Follow official Docker Engine installation docs for your distribution." >&2
    return 1
  fi
  if ! docker compose version >/dev/null 2>&1; then
    echo "Docker Compose plugin is not available. Install the official Docker Compose plugin." >&2
    return 1
  fi
  if [ "$PUBLIC_EXPOSURE" = "true" ] && [ -f "${INSTALL_DIR}/docker-compose.public.yml" ]; then
    run docker compose -f "${INSTALL_DIR}/docker-compose.yml" -f "${INSTALL_DIR}/docker-compose.public.yml" "$@"
  else
    run docker compose -f "${INSTALL_DIR}/docker-compose.yml" "$@"
  fi
}

docker_install() {
  need_root
  write_compose_files
  compose up -d
  docker_status
}

docker_status() {
  compose ps
}

docker_logs() {
  compose logs --tail=120 aimilivpn
}

uninstall_native() {
  need_root
  service_action stop || true
  if command -v systemctl >/dev/null 2>&1; then
    run systemctl disable aimilivpn.service || true
    run rm -f /etc/systemd/system/aimilivpn.service
    run systemctl daemon-reload
  elif command -v rc-service >/dev/null 2>&1; then
    run rc-update del aimilivpn default || true
    run rm -f /etc/init.d/aimilivpn
  fi
  run rm -f /usr/bin/ml
  run rm -rf "${INSTALL_DIR}/bin"
  maybe_delete_data
}

uninstall_docker() {
  need_root
  if [ -f "${INSTALL_DIR}/docker-compose.yml" ]; then
    compose down
  fi
  maybe_delete_data
}

maybe_delete_data() {
  if [ "$DELETE_DATA" != "true" ]; then
    echo "Persistent data preserved at ${DATA_DIR}. Use --delete-data to remove it."
    return 0
  fi
  if confirm "Delete persistent AimiliVPN data at ${DATA_DIR}?"; then
    run rm -rf "$DATA_DIR"
  else
    echo "Data preserved at ${DATA_DIR}."
  fi
}

case "${ACTION}:${MODE}" in
  install:native|upgrade:native) native_install ;;
  install:docker|upgrade:docker) docker_install ;;
  status:native) native_status ;;
  status:docker) docker_status ;;
  logs:native) native_logs ;;
  logs:docker) docker_logs ;;
  start:native|stop:native|restart:native) service_action "$ACTION" ;;
  start:docker) compose up -d ;;
  stop:docker) compose stop ;;
  restart:docker) compose restart ;;
  uninstall:native) uninstall_native ;;
  uninstall:docker) uninstall_docker ;;
  *) echo "Unsupported action/mode: ${ACTION}/${MODE}" >&2; exit 2 ;;
esac
