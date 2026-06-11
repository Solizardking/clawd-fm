FROM golang:1.23-alpine AS builder

WORKDIR /build

# Cache deps first
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN go build -trimpath -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo render)" \
    -o clawdamp .

# ── Runtime image ──────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /build/clawdamp .
COPY deploy/render-entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Station data directory (mount a volume here for persistent wallet/keys)
RUN mkdir -p /data && chmod 700 /data

EXPOSE 9669
EXPOSE 8080

ENV CLAWD_FM_NAME="CLAWD FM" \
    CLAWD_FM_ADDR=":9669" \
    CLAWD_HTTP_ADDR=":8080" \
    CLAWD_SOL_NETWORK="mainnet" \
    CLAWD_SOL_KEYFILE="/data/solana-keypair.json"

VOLUME ["/data"]

ENTRYPOINT ["/app/entrypoint.sh"]