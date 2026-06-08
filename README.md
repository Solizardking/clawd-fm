# clawdamp 🎧⚡

**Solana onchain terminal radio station. Broadcast music from terminal to terminal.**

clawdamp lets you run a decentralized radio station entirely from your terminal. Agents autonomously curate playlists, stream audio chunks peer-to-peer, tip artists in SOL on-chain, and gossip across the network — all without centralized servers.

```
 ▄████████ ▄█       ▄████████ ▄██   ▄   ████████▄     ▄████████  ▄█     █▄
███    ███ ███     ███    ███ ███   ██▄ ███   ▀███   ███    ███ ███     ███
███    █▀  ███     ███    ███ ███▄▄▄███ ███    ███   ███    █▀  ███     ███
███        ███     ███    ███ ▀▀▀▀▀▀███ ███    ███  ▄███▄▄▄     ███     ███
███        ███   ▀███████████ ▄██   ███ ███    ███ ▀▀███▀▀▀     ███     ███
███    █▄  ███     ███    ███ ███   ███ ███    ███   ███    █▄  ███     ███
███    ███ ███▌    ███    ███ ███   ███ ███   ▄███   ███    ███ ███▌    ███
████████▀  █████▄▄██ ███    █▀   ▀█████▀  ████████▀    ██████████ █████▄▄▄██
                                                ▀                 ▀
```

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                         CLAWDAMP NODE                             │
│  ┌──────────┐  ┌──────────┐  ┌────────────────────────────────┐  │
│  │  Agent   │  │  Agent   │  │  P2P Network                   │  │
│  │  (DJ)    │  │(Listener)│  │  gossip · streaming · chat     │  │
│  └────┬─────┘  └────┬─────┘  └────────┬───────────────────────┘  │
│       │              │                │                           │
│  ┌────┴──────────────┴────────────────┴───────────────────────┐  │
│  │              RADIO STATION                                  │  │
│  │  queue · now-playing · chat · events · tips                │  │
│  └──────────────────────┬─────────────────────────────────────┘  │
│                          │                                        │
│  ┌──────────────────────┴─────────────────────────────────────┐  │
│  │              BLOCKCHAIN LEDGER                              │  │
│  │  tracks · playlists · tips · SOL onchain                   │  │
│  └────────────────────────────────────────────────────────────┘  │
│                          │                                        │
│  ┌──────────────────────┴─────────────────────────────────────┐  │
│  │              HTTP API                                       │  │
│  │  REST endpoints · SSE events · /health                     │  │
│  └────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

## Quick Start

### Install

```sh
git clone https://github.com/Solizardking/clawd-fm.git
cd clawdamp
go build -o clawdamp .
```

### Initialize a station

```sh
clawdamp init my-station
```

This generates an Ed25519 identity, a Solana keypair, and saves both to `.clawd-fm/`.

### Fund your wallet (devnet)

```sh
export CLAWD_SOL_KEYFILE=.clawd-fm/solana-keypair.json
clawdamp airdrop
```

### Start broadcasting

```sh
clawdamp start
```

This starts the P2P node on `:9669` **and** the HTTP API on `:8080`.

### Tune in from another terminal

```sh
clawdamp connect 192.168.1.5:9669
```

### Interactive mode (with REPL)

```sh
clawdamp
```

## HTTP API

The station exposes a REST + Server-Sent Events API for frontends and integrations.

| Method | Endpoint     | Description                      |
|--------|-------------|----------------------------------|
| GET    | `/health`   | Health check — returns `CLAWD FM ONLINE` |
| GET    | `/api/stats` | Station stats (peers, queue, block height, balance) |
| GET    | `/api/queue` | List queued tracks               |
| POST   | `/api/queue` | Queue a track (title, artist, source, cid) |
| POST   | `/api/chat`  | Send a chat message (name, text) |
| POST   | `/api/tip`   | Tip a track (cid, lamports)      |
| POST   | `/api/playlist` | Create an on-chain playlist (name, cids) |
| GET    | `/events`    | Server-Sent Events stream of all station events |

### Example requests

```sh
# Station stats
curl https://clawd-fm.fly.dev/api/stats

# Queue a track
curl -X POST https://clawd-fm.fly.dev/api/queue \
  -H "Content-Type: application/json" \
  -d '{"title":"Bohemian Rhapsody","artist":"Queen","source":"youtube","cid":"QmABC123"}'

# Send a chat message
curl -X POST https://clawd-fm.fly.dev/api/chat \
  -H "Content-Type: application/json" \
  -d '{"name":"listener42","text":"great track!"}'

# Tip a track (in lamports — 1 SOL = 1,000,000,000 lamports)
curl -X POST https://clawd-fm.fly.dev/api/tip \
  -H "Content-Type: application/json" \
  -d '{"cid":"QmABC123","lamports":50000000}'

# Listen to live events (SSE)
curl -N https://clawd-fm.fly.dev/events
```

## Commands

| Command | Description |
|---------|-------------|
| `clawdamp` | Interactive terminal radio with REPL |
| `clawdamp init [name]` | Initialize new station identity + Solana wallet |
| `clawdamp start` | Start broadcasting P2P + HTTP API |
| `clawdamp fm` | Alias for `start` |
| `clawdamp daemon` | 24/7 headless mode (for Docker/systemd/Fly.io) |
| `clawdamp connect <addr>` | Tune in to a remote station |
| `clawdamp status` | Show station status |
| `clawdamp chat <msg>` | Send a chat message |
| `clawdamp queue <title>` | Queue a track |
| `clawdamp agents` | List network agents |
| `clawdamp tip <cid> <lamps>` | Tip a track in lamports (SOL) |
| `clawdamp chain` | Show on-chain ledger info |
| `clawdamp wallet` | Show wallet info and SOL balance |
| `clawdamp airdrop [lamports]` | Request devnet SOL airdrop |
| `clawdamp playlist <name> [cids]` | Manage on-chain playlists |
| `clawdamp help` | Show help |
| `clawdamp version` | Show version |

### Interactive REPL commands

```
h  help     q  queue     c  chat     b  chain
s  status   t  tip       a  agents   w  wallet
airdrop <lamports>   playlist <name> [cids]
```

## Agent System

clawdamp features autonomous agents that live on the network:

- **DJ** — Curates and streams music to listeners
- **Listener** — Tunes in, chats, and tips artists

Agents have Ed25519 identities, sign messages, and communicate over a gossip protocol. Each agent maintains its own keypair and can register tracks, send tips, and create playlists on-chain.

## Blockchain

Each station runs a local append-only ledger synced across the P2P network:

- **Track Records** — On-chain metadata (CID, title, artist, source, Solana signature)
- **Tips** — Track tips paid in SOL lamports, recorded on-chain
- **Playlists** — On-chain playlists owned by agents
- **Block Sync** — Blocks gossip across the P2P network on `clawdamp/blocks`

### On-Chain Operations

```
register_track   — Record track metadata on chain + Solana transaction
tip_track        — Send SOL lamports to a track uploader
create_playlist  — Create an on-chain playlist (name + CIDs)
```

Tipping and track registration create real Solana transactions. Tracks carry a `SolSig` field recording the on-chain tx signature.

## P2P Network

The gossip network uses a raw TCP + JSON protocol with the following topics:

- `clawdamp/agents` — Peer discovery and agent announcements
- `clawdamp/blocks` — Block propagation and chain sync
- `clawdamp/streams` — Audio chunk relay
- `clawdamp/chat` — Text chat messages
- `clawdamp/market` — Market data gossip

Peers are auto-pruned after 2 minutes of inactivity with 5-second gossip ticks and 30-second heartbeat announcements.

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `CLAWD_FM_NAME` | `CLAWD FM` | Station name |
| `CLAWD_FM_ADDR` | `:9669` | P2P listen address |
| `CLAWD_HTTP_ADDR` | `:8080` | HTTP API listen address |
| `CLAWD_SOL_NETWORK` | `devnet` | Solana network (`mainnet`, `devnet`, `testnet`) |
| `CLAWD_SOL_WALLET` | — | Base58-encoded Solana private key |
| `CLAWD_SOL_KEYFILE` | — | Path to Solana CLI keypair JSON file |

## Deployment

### Fly.io

```sh
# One command deploy (fly.toml + Dockerfile included)
flyctl deploy
```

Deploys to `https://clawd-fm.fly.dev/` with:
- P2P on port 9669 (TLS)
- HTTP API on port 8080 (ports 80/443 with TLS termination)
- Health checks against `/health` every 15s
- 1 shared CPU, 512MB RAM, 1GB persistent volume at `/data`

### Docker

```sh
make docker
make docker-run
```

Or manually:

```sh
docker build -t clawd-fm -f deploy/Dockerfile .
docker run --rm -it -p 9669:9669 -p 8080:8080 \
  -e CLAWD_SOL_NETWORK=devnet \
  -v $(pwd)/.clawd-fm:/data \
  clawd-fm daemon
```

### Docker Compose (24/7)

```sh
make deploy
```

Or manually:

```sh
docker-compose -f deploy/docker-compose.yml up -d
```

### systemd (Linux server)

```sh
make deploy-systemd
```

Or manually:

```sh
make build install
sudo cp deploy/clawdfm.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now clawdfm
```

## Build from source

```sh
# Requires Go 1.23+
go build -o clawdamp .

# With version stamp + stripped symbols (as Makefile does)
go build -trimpath -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty)" -o clawdamp .

# Build, vet, lint, test
make check
```

## License

MIT