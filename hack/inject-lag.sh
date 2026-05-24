#!/usr/bin/env bash
# Flip simulated cache staleness on a running controller.
# Usage: ./inject-lag.sh <window> <seconds> [host:port]
#   window  = how many recent writes are hidden from the cache view
#   seconds = simulated photo-age (for the lag metric)
# Example: ./inject-lag.sh 3 2.0          # hide 3 recent writes, ~2s stale
#          ./inject-lag.sh 0 0            # clear lag (cache fresh again)
set -euo pipefail
WINDOW="${1:-3}"; SECONDS_LAG="${2:-2.0}"; HOST="${3:-localhost:9090}"
curl -sf -X POST "http://${HOST}/lag" \
  -H 'Content-Type: application/json' \
  -d "{\"window\":${WINDOW},\"seconds\":${SECONDS_LAG}}" \
  && echo "injected: window=${WINDOW} seconds=${SECONDS_LAG} -> ${HOST}"
