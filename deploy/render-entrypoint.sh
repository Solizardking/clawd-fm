#!/bin/sh
# Render entrypoint — maps $PORT to CLAWD_HTTP_ADDR if the env var
# isn't already explicitly set by the user.
#
# Render injects $PORT automatically. We forward it to the Go binary
# which reads CLAWD_HTTP_ADDR.

if [ -n "$PORT" ] && [ -z "$CLAWD_HTTP_ADDR" ]; then
  export CLAWD_HTTP_ADDR=":$PORT"
fi

exec /app/clawdamp daemon