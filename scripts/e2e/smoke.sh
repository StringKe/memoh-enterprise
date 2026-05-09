#!/usr/bin/env bash
set -euo pipefail

SERVER_URL="${MEMOH_E2E_SERVER_URL:-http://127.0.0.1:26810}"
WEB_URL="${MEMOH_E2E_WEB_URL:-http://127.0.0.1:26811}"
WAIT_SECONDS="${MEMOH_E2E_WAIT_SECONDS:-180}"
CHECK_CONNECT_CHAT=false

usage() {
  cat <<'EOF'
Usage: scripts/e2e/smoke.sh [--connect-chat]

Environment:
  MEMOH_E2E_SERVER_URL   Server URL. Default: http://127.0.0.1:26810
  MEMOH_E2E_WEB_URL      Web management UI URL. Default: http://127.0.0.1:26811
  MEMOH_E2E_WAIT_SECONDS Wait timeout. Default: 180
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --connect-chat)
      CHECK_CONNECT_CHAT=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

wait_for() {
  name="$1"
  url="$2"
  deadline=$((SECONDS + WAIT_SECONDS))
  while [ "$SECONDS" -lt "$deadline" ]; do
    if curl -fsS "$url" >/tmp/memoh-e2e-response.json 2>/tmp/memoh-e2e-error.log; then
      echo "[e2e] $name is ready"
      return 0
    fi
    sleep 2
  done
  echo "[e2e] $name did not become ready: $url" >&2
  cat /tmp/memoh-e2e-error.log >&2 || true
  exit 1
}

require curl

wait_for "server" "$SERVER_URL/ping"
if ! grep -q '"status"[[:space:]]*:[[:space:]]*"ok"' /tmp/memoh-e2e-response.json; then
  echo "[e2e] server ping response is not ok" >&2
  cat /tmp/memoh-e2e-response.json >&2
  exit 1
fi

wait_for "web management UI" "$WEB_URL/health"
if ! grep -q '^ok' /tmp/memoh-e2e-response.json; then
  echo "[e2e] web health response is not ok" >&2
  cat /tmp/memoh-e2e-response.json >&2
  exit 1
fi

if [ "$CHECK_CONNECT_CHAT" = true ]; then
  curl -fsS \
    -H "Content-Type: application/json" \
    -H "Connect-Protocol-Version: 1" \
    --data '{"botId":"e2e-bot","sessionId":"e2e-session","message":"ping"}' \
    "$SERVER_URL/connect/memoh.private.v1.ChatService/StreamChat" \
    >/tmp/memoh-e2e-connect-chat.json \
    || {
      echo "[e2e] connect chat stream failed" >&2
      cat /tmp/memoh-e2e-connect-chat.json >&2 || true
      exit 1
    }
fi

echo "[e2e] smoke passed"
