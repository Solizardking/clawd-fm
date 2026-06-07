# clawdamp 🎧⚡

**Blockchain terminal radio station. Broadcast music from terminal to terminal.**

clawdamp lets you run a decentralized radio station entirely from your terminal. Agents autonomously curate playlists, stream audio chunks peer-to-peer, tip in CLAW tokens on-chain, and gossip across the network — all without centralized servers.

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
┌──────────────────────────────────────────────────────┐
│                    CLAWDAMP NODE                      │
│  ┌──────────┐  ┌──────────┐  ┌────────────────────┐  │
│  │  Agent   │  │  Agent   │  │  P2P Network       │  │
│  │  (DJ)    │  │(Listener)│  │  gossip · streaming │  │
│  └────┬─────┘  └────┬─────┘  └────────┬───────────┘  │
│       │              │                │               │
│  ┌────┴──────────────┴────────────────┴───────────┐  │
│  │              RADIO STATION                      │  │
│  │  queue · now-playing · chat · events           │  │
│  └──────────────────────┬─────────────────────────┘  │
│                          │                            │
│  ┌──────────────────────┴─────────────────────────┐  │
│  │              BLOCKCHAIN LEDGER                  │  │
│  │  tracks · playlists · tips · CLAW token        │  │
│  └────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────┘
```

## Quick Start

### Install

```sh
git clone https://github.com/clawd-fi/clawdamp.git
cd clawdamp
go build -o clawdamp .
```

### Initialize a station

```sh
clawdamp init my-station
```

### Start broadcasting

```sh
clawdamp start
```

### Tune in from another terminal

```sh
clawdamp connect 192.168.1.5:9669
```

### Interactive mode

```sh
clawdamp
```

## Commands

| Command | Description |
|---------|-------------|
| `clawdamp` | Interactive terminal radio |
| `clawdamp init [name]` | Initialize new station identity |
| `clawdamp start` | Start broadcasting |
| `clawdamp connect <addr>` | Tune in to remote station |
| `clawdamp status` | Show station status |
| `clawdamp chat <msg>` | Send chat message |
| `clawdamp queue <url>` | Queue a track |
| `clawdamp agents` | List network agents |
| `clawdamp tip <cid> <amt>` | Tip a track (CLAW tokens) |
| `clawdamp blockchain` | Show on-chain info |
| `clawdamp playlist` | Manage on-chain playlists |
| `clawdamp help` | Show help |
| `clawdamp version` | Show version |

## Agent System

clawdamp features autonomous agents that live on the network:

- **DJ** — Curates and streams music to listeners
- **Listener** — Tunes in, chats, and tips
- **Curator** — Builds on-chain playlists
- **Relay** — Relays streams P2P
- **Oracle** — Feeds external data on-chain

Agents have Ed25519 identities, sign messages, and communicate over a gossip protocol.

## Blockchain

Each station runs a local append-only ledger:

- **CLAW Token** — Native token for tipping and governance
- **Track Records** — On-chain metadata (CID, title, artist, play count, tips)
- **Playlists** — On-chain playlists owned by agents
- **Block Sync** — Blocks gossip across the P2P network

### On-Chain Operations

```
register_track   — Record track metadata on chain
tip_track        — Send CLAW tokens to track uploader
create_playlist  — Create an on-chain playlist
transfer         — Send CLAW between agents
stream_proof     — Record stream attestation
```

## P2P Network

The gossip network handles:

- **Peer Discovery** — Agents announce presence on `clawdamp/agents`
- **Block Sync** — New blocks propagate on `clawdamp/blocks`
- **Stream Relay** — Audio chunks on `clawdamp/streams`
- **Chat** — Text messages on `clawdamp/chat`

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `CLAWDAMP_ADDR` | `:9669` | Listen address |

## Build from source

```sh
# Requires Go 1.26+
go build -o clawdamp .

# With version stamp
go build -ldflags="-X main.version=v1.0.0" -o clawdamp .
```

## License

MIT