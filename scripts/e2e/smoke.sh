#!/usr/bin/env bash
set -euo pipefail

SERVER_URL="${MEMOH_E2E_SERVER_URL:-http://127.0.0.1:8080}"
BROWSER_URL="${MEMOH_E2E_BROWSER_URL:-http://127.0.0.1:8083}"
WAIT_SECONDS="${MEMOH_E2E_WAIT_SECONDS:-180}"
CHECK_BROWSER=false

usage() {
  cat <<'EOF'
Usage: scripts/e2e/smoke.sh [--browser]

Environment:
  MEMOH_E2E_SERVER_URL   Server URL. Default: http://127.0.0.1:8080
  MEMOH_E2E_BROWSER_URL  Browser Gateway URL. Default: http://127.0.0.1:8083
  MEMOH_E2E_WAIT_SECONDS Wait timeout. Default: 180
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --browser)
      CHECK_BROWSER=true
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

curl -fsS "$SERVER_URL/api/swagger.json" >/tmp/memoh-e2e-swagger.json
if ! grep -q '"swagger"[[:space:]]*:[[:space:]]*"2.0"' /tmp/memoh-e2e-swagger.json; then
  echo "[e2e] swagger endpoint did not return Swagger 2.0 JSON" >&2
  head -c 500 /tmp/memoh-e2e-swagger.json >&2
  echo >&2
  exit 1
fi
echo "[e2e] swagger endpoint is ready"

if [ "$CHECK_BROWSER" = true ]; then
  wait_for "browser gateway" "$BROWSER_URL/health"
  if ! grep -q '"status"[[:space:]]*:[[:space:]]*"ok"' /tmp/memoh-e2e-response.json; then
    echo "[e2e] browser health response is not ok" >&2
    cat /tmp/memoh-e2e-response.json >&2
    exit 1
  fi
fi

echo "[e2e] smoke passed"
