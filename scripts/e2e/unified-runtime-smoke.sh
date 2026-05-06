#!/usr/bin/env bash
set -euo pipefail

SERVER_URL="${MEMOH_E2E_SERVER_URL:-http://127.0.0.1:26810}"
WEB_URL="${MEMOH_E2E_WEB_URL:-http://127.0.0.1:26811}"
BROWSER_URL="${MEMOH_E2E_BROWSER_URL:-http://127.0.0.1:26812}"
RUNNER_URL="${MEMOH_E2E_RUNNER_URL:-http://127.0.0.1:26813}"
CONNECTOR_URL="${MEMOH_E2E_CONNECTOR_URL:-http://127.0.0.1:26814}"
INTEGRATION_URL="${MEMOH_E2E_INTEGRATION_URL:-http://127.0.0.1:26815}"
WORKER_URL="${MEMOH_E2E_WORKER_URL:-http://127.0.0.1:26816}"
WAIT_SECONDS="${MEMOH_E2E_WAIT_SECONDS:-180}"

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
    if curl -fsS "$url" >/tmp/memoh-e2e-unified-body 2>/tmp/memoh-e2e-unified-error; then
      echo "[e2e] $name is ready"
      return 0
    fi
    sleep 2
  done
  echo "[e2e] $name did not become ready: $url" >&2
  cat /tmp/memoh-e2e-unified-error >&2 || true
  exit 1
}

require curl

wait_for "server" "$SERVER_URL/ping"
wait_for "web" "$WEB_URL/health"
wait_for "browser gateway" "$BROWSER_URL/health"
wait_for "agent runner" "$RUNNER_URL/health"
wait_for "connector" "$CONNECTOR_URL/health"
wait_for "integration gateway" "$INTEGRATION_URL/health"
wait_for "worker" "$WORKER_URL/health"

echo "[e2e] unified runtime smoke passed"
