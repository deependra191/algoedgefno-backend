#!/bin/bash
set -euo pipefail

# Expected: /opt/algoedgefno/env/telegram.env owned by root, chmod 600,
# containing TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID.
# shellcheck source=/dev/null
source /opt/algoedgefno/env/telegram.env
: "${TELEGRAM_BOT_TOKEN:?telegram.env must set TELEGRAM_BOT_TOKEN}"
: "${TELEGRAM_CHAT_ID:?telegram.env must set TELEGRAM_CHAT_ID}"

# HC_PING_NOTIFY_PROD is optional — omitting it skips the HC ping.
# Omitting it in production means the prod-alert-cron HC check will alert
# "no ping received" on schedule. See docs/monitoring-setup.md Phase 2.
# shellcheck source=/dev/null
source /opt/algoedgefno/env/healthchecks.env 2>/dev/null || true
HC_PING_NOTIFY_PROD="${HC_PING_NOTIFY_PROD:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=/dev/null
source "${SCRIPT_DIR}/monitoring/_hc-ping.sh"

COMPOSE_FILE="/opt/algoedgefno/compose/docker-compose.yml"

# Always remove the curl config temp file on exit (holds the bot token).
_cfg=""
cleanup() {
    [[ -n "${_cfg}" ]] && rm -f "${_cfg}"
    [[ -n "${_hc_cfg}" ]] && rm -f "${_hc_cfg}"
}
trap cleanup EXIT

html_escape() {
    local s="$1"
    s="${s//&/&amp;}"
    s="${s//</&lt;}"
    s="${s//>/&gt;}"
    printf '%s' "${s}"
}

# Writes the bot token to a temp config file (chmod 600) so it does not
# appear as a command-line argument in the process list.
send_telegram() {
    local message="$1"
    _cfg=$(mktemp)
    chmod 600 "${_cfg}"
    printf 'url = "https://api.telegram.org/bot%s/sendMessage"\n' "${TELEGRAM_BOT_TOKEN}" > "${_cfg}"
    curl -fsS --config "${_cfg}" \
        --data-urlencode "text=${message}" \
        -d "chat_id=${TELEGRAM_CHAT_ID}" \
        -d "parse_mode=HTML" > /dev/null
    rm -f "${_cfg}"
    _cfg=""
}

# ASCII unit separator (0x1f) — very unlikely to appear in sync error text.
sep=$'\x1f'

if ! result=$(docker compose -f "${COMPOSE_FILE}" exec -T postgres \
    psql -U algoedgefno_prod_app -d algoedgefno_prod -tA -F "${sep}" -c \
    "SELECT status, records_processed, started_at,
            COALESCE(completed_at::text, ''), COALESCE(error_message, '')
     FROM sync_runs
     WHERE started_at > NOW() - INTERVAL '12 hours'
     ORDER BY started_at DESC
     LIMIT 1;" 2>&1); then
    send_telegram "$(printf '<b>⚠️ AlgoEdge Prod Sync — CHECK FAILED</b>\nCould not query sync_runs.\n<code>%s</code>' \
        "$(html_escape "${result}")")"
    exit 1
fi

if [[ -z "${result}" ]]; then
    send_telegram "$(printf '<b>⚠️ AlgoEdge Prod Sync — NO RUN</b>\nNo sync_runs row found in the last 12 hours.\nExpected window: 00:45 IST daily.')"
    exit 0
fi

IFS="${sep}" read -r status records started completed error_msg <<< "${result}"

started_fmt=$(TZ="Asia/Kolkata" date -d "${started}" +"%Y-%m-%d %H:%M IST" 2>/dev/null || printf '%s' "${started}")
completed_fmt=$(TZ="Asia/Kolkata" date -d "${completed}" +"%Y-%m-%d %H:%M IST" 2>/dev/null || printf '%s' "${completed}")

if [[ "${status}" == "COMPLETED" ]]; then
    send_telegram "$(printf '<b>✅ AlgoEdge Prod Sync — SUCCESS</b>\nRecords: %s\nStarted: %s\nCompleted: %s' \
        "$(html_escape "${records}")" \
        "$(html_escape "${started_fmt}")" \
        "$(html_escape "${completed_fmt}")")"
    # HC ping on success only — the Telegram alert above is already the failure
    # path; pinging /fail here too would produce duplicate operator alerts.
    hc_ping "${HC_PING_NOTIFY_PROD}" ""
else
    send_telegram "$(printf '<b>❌ AlgoEdge Prod Sync — %s</b>\nRecords: %s\nStarted: %s\nError: %s' \
        "$(html_escape "${status}")" \
        "$(html_escape "${records}")" \
        "$(html_escape "${started_fmt}")" \
        "$(html_escape "${error_msg:-none}")")"
fi
