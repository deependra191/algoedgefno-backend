#!/bin/bash
set -euo pipefail

set -a
# shellcheck source=/dev/null
source /opt/algoedgefno/env/telegram.env
set +a

COMPOSE_FILE="/opt/algoedgefno/compose/docker-compose.yml"
TELEGRAM_TOKEN="${TELEGRAM_BOT_TOKEN}"
TELEGRAM_CHAT_ID="${TELEGRAM_CHAT_ID}"

# Writes the API URL (which contains the bot token) to a temp config file so
# the token does not appear in the process list via ps aux.
send_telegram() {
    local message="$1"
    local cfg
    cfg=$(mktemp)
    chmod 600 "${cfg}"
    printf 'url = "https://api.telegram.org/bot%s/sendMessage"\n' "${TELEGRAM_TOKEN}" > "${cfg}"
    curl -s --config "${cfg}" \
        --data-urlencode "text=${message}" \
        -d "chat_id=${TELEGRAM_CHAT_ID}" \
        -d "parse_mode=HTML" > /dev/null
    rm -f "${cfg}"
}

# Uses ~~ as separator; | is too common in error messages.
result=$(docker compose -f "${COMPOSE_FILE}" exec -T postgres \
    psql -U algoedgefno_staging_app -d algoedgefno_staging -tA -c \
    "SELECT status || '~~' || records_processed || '~~' || started_at || '~~' \
            || COALESCE(completed_at::text, '') || '~~' || COALESCE(error_message, '')
     FROM sync_runs
     WHERE started_at > NOW() - INTERVAL '12 hours'
     ORDER BY started_at DESC
     LIMIT 1;")

if [[ -z "$result" ]]; then
    send_telegram "$(printf '<b>⚠️ AlgoEdge Staging Sync — NO RUN</b>\nNo sync_runs row found in the last 12 hours.\nExpected window: 18:45 UTC daily.')"
    exit 0
fi

IFS='~~' read -r status records started completed error_msg <<< "$result"

started_fmt=$(date -d "${started}" +"%Y-%m-%d %H:%M UTC" 2>/dev/null || echo "${started}")
completed_fmt=$(date -d "${completed}" +"%Y-%m-%d %H:%M UTC" 2>/dev/null || echo "${completed}")

if [[ "$status" == "COMPLETED" ]]; then
    send_telegram "$(printf '<b>✅ AlgoEdge Staging Sync — SUCCESS</b>\nRecords: %s\nStarted: %s\nCompleted: %s' \
        "${records}" "${started_fmt}" "${completed_fmt}")"
else
    send_telegram "$(printf '<b>❌ AlgoEdge Staging Sync — %s</b>\nRecords: %s\nStarted: %s\nError: %s' \
        "${status}" "${records}" "${started_fmt}" "${error_msg:-none}")"
fi
