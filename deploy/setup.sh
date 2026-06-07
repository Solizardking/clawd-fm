#!/usr/bin/env bash
# CLAWD FM — First Solana Onchain Terminal Radio Station
# Quick setup script for 24/7 VPS/server deployment

set -euo pipefail

INSTALL_DIR="${CLAWD_INSTALL_DIR:-/opt/clawd-fm}"
SERVICE_USER="${CLAWD_USER:-clawd}"
BINARY="clawdamp"

echo "=== CLAWD FM Setup ==="
echo "Install dir: $INSTALL_DIR"
echo "Service user: $SERVICE_USER"
echo

# 1. Install Go if not present
if ! command -v go &>/dev/null; then
    echo "[setup] Installing Go..."
    curl -fsSL https://go.dev/dl/go1.23.linux-amd64.tar.gz | tar -C /usr/local -xz
    export PATH="$PATH:/usr/local/go/bin"
fi

echo "[setup] Go: $(go version)"

# 2. Build the binary
echo "[setup] Building CLAWD FM..."
go build -trimpath -ldflags="-s -w" -o $BINARY .
echo "[setup] Binary: ./$BINARY"

# 3. Create service user and install dir
if ! id "$SERVICE_USER" &>/dev/null; then
    echo "[setup] Creating user: $SERVICE_USER"
    useradd -r -m -d "$INSTALL_DIR" -s /sbin/nologin "$SERVICE_USER" || true
fi

mkdir -p "$INSTALL_DIR/.clawd-fm"
install -m 755 "$BINARY" "$INSTALL_DIR/$BINARY"
chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"

# 4. Generate Solana wallet if none exists
KEYFILE="$INSTALL_DIR/.clawd-fm/solana-keypair.json"
if [[ ! -f "$KEYFILE" ]]; then
    echo "[setup] Generating Solana wallet..."
    sudo -u "$SERVICE_USER" "$INSTALL_DIR/$BINARY" init clawd-fm
    echo "[setup] Wallet saved to: $KEYFILE"
else
    echo "[setup] Wallet already exists: $KEYFILE"
fi

# 5. Write environment file
ENV_FILE="/etc/clawd-fm.env"
cat > "$ENV_FILE" <<EOF
CLAWD_FM_NAME=CLAWD FM
CLAWD_FM_ADDR=:9669
CLAWD_SOL_NETWORK=mainnet
CLAWD_SOL_KEYFILE=$KEYFILE
EOF
chmod 640 "$ENV_FILE"
echo "[setup] Env file: $ENV_FILE"

# 6. Install systemd service
install -m 644 deploy/clawdfm.service /etc/systemd/system/
sed -i "s|/opt/clawd-fm|$INSTALL_DIR|g" /etc/systemd/system/clawdfm.service
sed -i "s|User=clawd|User=$SERVICE_USER|g" /etc/systemd/system/clawdfm.service
systemctl daemon-reload
systemctl enable clawdfm
systemctl restart clawdfm

echo
echo "=== CLAWD FM is LIVE ==="
echo "Status: systemctl status clawdfm"
echo "Logs:   journalctl -fu clawdfm"
echo "P2P:    :9669"
echo
echo "To fund your Solana wallet on mainnet, send SOL to:"
"$INSTALL_DIR/$BINARY" wallet
