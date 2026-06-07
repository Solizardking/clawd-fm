#!/usr/bin/env bash
# CLAWD FM — Quick Bootstrap
# Run this from your terminal to fetch Go dependencies and build.
set -euo pipefail

echo "=== CLAWD FM Bootstrap ==="
echo

# Check Go
if ! command -v go &>/dev/null; then
    echo "ERROR: Go not found. Install from https://go.dev/dl/"
    exit 1
fi
echo "Go: $(go version)"

cd "$(dirname "$0")"
echo "Dir: $(pwd)"
echo

# Fetch dependencies (includes gagliardetto/solana-go)
echo "[1/3] Fetching dependencies (solana-go + deps)..."
go mod tidy
echo "      Done."

# Build CLAWD FM binary
echo "[2/3] Building clawdamp binary..."
go build -trimpath -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o clawdamp .
echo "      Done: ./clawdamp"

# Init station (wallet + identity)
echo "[3/3] Initializing CLAWD FM station..."
./clawdamp init clawd-fm
echo "      Done."

echo
echo "=== CLAWD FM Ready ==="
echo
echo "  Run interactively:   ./clawdamp"
echo "  Start broadcasting:  ./clawdamp start"
echo "  24/7 daemon:         ./clawdamp daemon"
echo "  Fund wallet (dev):   ./clawdamp airdrop"
echo "  Docker 24/7:         docker-compose -f deploy/docker-compose.yml up -d"
echo
echo "Environment:"
echo "  export CLAWD_SOL_NETWORK=devnet"
echo "  export CLAWD_SOL_KEYFILE=.clawd-fm/solana-keypair.json"
echo "  export CLAWD_FM_NAME='CLAWD FM'"
echo "  export CLAWD_FM_ADDR=':9669'"
