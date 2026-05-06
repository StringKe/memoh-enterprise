#!/usr/bin/env bash
set -euo pipefail

CONNECT_URL="${MEMOH_E2E_CONNECT_URL:-${MEMOH_E2E_SERVER_URL:-http://127.0.0.1:26810}/connect}"
WS_URL="${MEMOH_E2E_INTEGRATION_WS_URL:-ws://127.0.0.1:26815/integration/v1/ws}"
USERNAME="${MEMOH_E2E_ADMIN_USERNAME:-admin}"
PASSWORD="${MEMOH_E2E_ADMIN_PASSWORD:-admin123}"

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

json_get() {
  python3 -c 'import json,sys; data=json.load(sys.stdin); cur=data
for key in sys.argv[1].split("."):
    cur=cur[key]
print(cur)' "$1"
}

connect_rpc() {
  service="$1"
  method="$2"
  body="$3"
  auth_header=()
  if [ "${ACCESS_TOKEN:-}" != "" ]; then
    auth_header=(-H "Authorization: Bearer ${ACCESS_TOKEN}")
  fi
  curl -fsS \
    -H "Content-Type: application/json" \
    -H "Connect-Protocol-Version: 1" \
    "${auth_header[@]}" \
    --data "$body" \
    "${CONNECT_URL}/${service}/${method}"
}

require curl
require node
require python3

login_response="$(connect_rpc memoh.private.v1.AuthService Login "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
ACCESS_TOKEN="$(printf '%s' "$login_response" | json_get accessToken)"

bot_response="$(connect_rpc memoh.private.v1.BotService CreateBot "{\"displayName\":\"e2e-integration-bot\",\"metadata\":{}}")"
BOT_ID="$(printf '%s' "$bot_response" | json_get bot.id)"

token_response="$(connect_rpc memoh.private.v1.IntegrationAdminService CreateIntegrationApiToken \
  "{\"name\":\"e2e-integration\",\"scopeType\":\"global\",\"allowedEventTypes\":[\"bot.message\"],\"allowedActionTypes\":[\"subscribe\",\"send_message\",\"create_session\",\"get_session_status\",\"get_bot_status\"]}")"
RAW_TOKEN="$(printf '%s' "$token_response" | json_get rawToken)"
TOKEN_ID="$(printf '%s' "$token_response" | json_get token.id)"

MEMOH_E2E_WS_URL="$WS_URL" MEMOH_E2E_RAW_TOKEN="$RAW_TOKEN" MEMOH_E2E_BOT_ID="$BOT_ID" node <<'NODE'
const url = process.env.MEMOH_E2E_WS_URL;
const token = process.env.MEMOH_E2E_RAW_TOKEN;
const botId = process.env.MEMOH_E2E_BOT_ID;

function frame(correlationId, payload) {
  return JSON.stringify({
    version: "2026-05-05",
    messageId: crypto.randomUUID(),
    correlationId,
    ...payload,
  });
}

function waitFor(ws, predicate, timeoutMs = 10000) {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      ws.removeEventListener("message", onMessage);
      reject(new Error("timed out waiting for websocket frame"));
    }, timeoutMs);
    function onMessage(event) {
      const data = JSON.parse(event.data);
      if (data.error) {
        clearTimeout(timeout);
        ws.removeEventListener("message", onMessage);
        reject(new Error(`${data.error.code}: ${data.error.message}`));
        return;
      }
      if (predicate(data)) {
        clearTimeout(timeout);
        ws.removeEventListener("message", onMessage);
        resolve(data);
      }
    }
    ws.addEventListener("message", onMessage);
  });
}

const ws = new WebSocket(url);
await new Promise((resolve, reject) => {
  ws.addEventListener("open", resolve, { once: true });
  ws.addEventListener("error", reject, { once: true });
});

const authWait = waitFor(ws, (data) => data.authResponse);
ws.send(frame("auth-1", { authRequest: { token } }));
await authWait;

const subscribeWait = waitFor(ws, (data) => data.correlationId === "sub-1" && data.subscribeResponse);
ws.send(frame("sub-1", { subscribeRequest: { eventTypes: ["bot.message"], botIds: [botId] } }));
await subscribeWait;

const sessionWait = waitFor(ws, (data) => data.correlationId === "session-1" && data.createSessionResponse);
ws.send(frame("session-1", { createSessionRequest: { botId, externalSessionId: "e2e-session" } }));
const session = await sessionWait;
const sessionId = session.createSessionResponse.sessionId;

const sendWait = waitFor(ws, (data) => data.correlationId === "send-1" && data.sendBotMessageResponse);
ws.send(frame("send-1", { sendBotMessageRequest: { botId, sessionId, text: "hello from e2e" } }));
await sendWait;

const statusWait = waitFor(ws, (data) => data.correlationId === "status-1" && data.getSessionStatusResponse);
ws.send(frame("status-1", { getSessionStatusRequest: { sessionId } }));
await statusWait;

const ackWait = waitFor(ws, (data) => data.correlationId === "ack-1" && data.ackResponse);
ws.send(frame("ack-1", { ackRequest: { eventId: "e2e-event" } }));
await ackWait;

ws.close(1000);
NODE

connect_rpc memoh.private.v1.IntegrationAdminService DisableIntegrationApiToken "{\"id\":\"${TOKEN_ID}\"}" >/dev/null
connect_rpc memoh.private.v1.BotService DeleteBot "{\"id\":\"${BOT_ID}\"}" >/dev/null

echo "[e2e] integration websocket passed"
