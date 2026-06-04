#!/usr/bin/env bash
set -euo pipefail

bash -n install.sh
bash install.sh status --mode native --dry-run
bash install.sh start --mode native --dry-run
bash install.sh install --mode docker --dry-run --public
bash install.sh uninstall --mode native --dry-run --delete-data -y
