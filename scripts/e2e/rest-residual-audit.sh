#!/usr/bin/env bash
set -euo pipefail

SERVER_URL="${MEMOH_E2E_SERVER_URL:-http://127.0.0.1:26810}"
WEB_URL="${MEMOH_E2E_WEB_URL:-http://127.0.0.1:26811}"

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

assert_status() {
  url="$1"
  expected="$2"
  status="$(curl -ksS -o /tmp/memoh-e2e-audit-body -w '%{http_code}' "$url" || true)"
  if [ "$status" != "$expected" ]; then
    echo "[e2e] ${url} returned ${status}, want ${expected}" >&2
    cat /tmp/memoh-e2e-audit-body >&2 || true
    exit 1
  fi
}

require curl

assert_status "$SERVER_URL/api/swagger.json" "404"
assert_status "$SERVER_URL/api/docs" "404"
assert_status "$SERVER_URL/connect/memoh.private.v1.AuthService/Login" "405"

assert_status "$SERVER_URL/ping" "200"
assert_status "$WEB_URL/health" "200"

# Runtime protocol endpoints remain intentionally outside the Connect management API.
runtime_patterns=(
  "/integration/v1/ws"
  "/oauth"
  "/webhook"
  "/bots/example/local/ws"
  "/bots/example/local/stream"
)

printf '%s\n' "${runtime_patterns[@]}" >/tmp/memoh-e2e-runtime-allowlist

echo "[e2e] rest residual audit passed"
