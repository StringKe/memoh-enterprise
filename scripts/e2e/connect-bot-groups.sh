#!/usr/bin/env bash
set -euo pipefail

CONNECT_URL="${MEMOH_E2E_CONNECT_URL:-${MEMOH_E2E_SERVER_URL:-http://127.0.0.1:26810}/connect}"
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
require python3

login_response="$(connect_rpc memoh.private.v1.AuthService Login "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
ACCESS_TOKEN="$(printf '%s' "$login_response" | json_get accessToken)"
if [ "$ACCESS_TOKEN" = "" ]; then
  echo "[e2e] login did not return an access token" >&2
  exit 1
fi

group_name="e2e-group-$(date +%s)"
group_response="$(connect_rpc memoh.private.v1.BotGroupService CreateBotGroup "{\"name\":\"${group_name}\",\"description\":\"connect e2e\",\"metadata\":{}}")"
GROUP_ID="$(printf '%s' "$group_response" | json_get group.id)"

connect_rpc memoh.private.v1.BotGroupService UpdateBotGroupSettings \
  "{\"groupId\":\"${GROUP_ID}\",\"settings\":{\"timezone\":\"UTC\",\"language\":\"en\",\"heartbeatEnabled\":true}}" >/tmp/memoh-e2e-group-settings.json

bot_response="$(connect_rpc memoh.private.v1.BotService CreateBot "{\"displayName\":\"e2e-bot\",\"groupId\":\"${GROUP_ID}\",\"metadata\":{}}")"
BOT_ID="$(printf '%s' "$bot_response" | json_get bot.id)"

settings_response="$(connect_rpc memoh.private.v1.SettingsService GetBotSettings "{\"botId\":\"${BOT_ID}\"}")"
timezone_source="$(printf '%s' "$settings_response" | python3 -c 'import json,sys
data=json.load(sys.stdin)
for source in data["settings"].get("sources", []):
    if source.get("field") == "timezone":
        print(source.get("source", ""))
        break')"
if [ "$timezone_source" != "bot_group" ]; then
  echo "[e2e] expected timezone source bot_group, got ${timezone_source:-empty}" >&2
  printf '%s\n' "$settings_response" >&2
  exit 1
fi

connect_rpc memoh.private.v1.SettingsService UpdateBotSettings \
  "{\"botId\":\"${BOT_ID}\",\"settings\":{\"timezone\":\"Europe/London\"},\"overrideMask\":{\"fields\":{\"timezone\":true}}}" >/tmp/memoh-e2e-bot-settings.json

connect_rpc memoh.private.v1.SettingsService RestoreBotSettingsInheritance \
  "{\"botId\":\"${BOT_ID}\",\"fields\":[\"timezone\"]}" >/tmp/memoh-e2e-restore.json

restored_source="$(cat /tmp/memoh-e2e-restore.json | python3 -c 'import json,sys
data=json.load(sys.stdin)
for source in data["settings"].get("sources", []):
    if source.get("field") == "timezone":
        print(source.get("source", ""))
        break')"
if [ "$restored_source" != "bot_group" ]; then
  echo "[e2e] expected restored timezone source bot_group, got ${restored_source:-empty}" >&2
  cat /tmp/memoh-e2e-restore.json >&2
  exit 1
fi

connect_rpc memoh.private.v1.BotService ClearBotGroup "{\"botId\":\"${BOT_ID}\"}" >/tmp/memoh-e2e-clear-group.json
connect_rpc memoh.private.v1.BotService AssignBotGroup "{\"botId\":\"${BOT_ID}\",\"groupId\":\"${GROUP_ID}\"}" >/tmp/memoh-e2e-assign-group.json

connect_rpc memoh.private.v1.BotService DeleteBot "{\"id\":\"${BOT_ID}\"}" >/dev/null
connect_rpc memoh.private.v1.BotGroupService DeleteBotGroup "{\"id\":\"${GROUP_ID}\"}" >/dev/null

echo "[e2e] connect bot groups passed"
