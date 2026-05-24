#!/usr/bin/env bash
set -euo pipefail

# Repo root = parent of scripts/ (clone dir name does not matter; works with sudo)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${INSTALL_DIR:-$(cd "${SCRIPT_DIR}/.." && pwd)}"
SERVICE_NAME="meshcore-bots"
UNIT_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
BINARY="${INSTALL_DIR}/meshcore-bots"
CONFIG="${INSTALL_DIR}/config.yaml"

if [[ -n "${SUDO_USER:-}" ]]; then
  SERVICE_USER="$SUDO_USER"
else
  SERVICE_USER="$(id -un)"
fi

if [[ ! -f "$BINARY" ]]; then
  echo "ERROR: binary not found at $BINARY"
  echo "  Build with: GOOS=linux GOARCH=amd64 go build -o meshcore-bots ./cmd/bots"
  exit 1
fi

if [[ ! -f "$CONFIG" ]]; then
  echo "ERROR: config.yaml not found at $CONFIG"
  echo "  Copy and edit: cp config.example.yaml config.yaml"
  exit 1
fi

echo "Installing systemd unit to $UNIT_FILE ..."
sudo tee "$UNIT_FILE" > /dev/null <<EOF
[Unit]
Description=MeshCore Morning Bots (Weather + Traffic + Tide + Sun + Frost)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${BINARY} --config ${CONFIG}
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now "$SERVICE_NAME"
echo ""
sudo systemctl status "$SERVICE_NAME" --no-pager
echo ""
echo "Done. Follow logs with:"
echo "  sudo journalctl -u $SERVICE_NAME -f"
