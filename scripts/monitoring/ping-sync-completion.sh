#!/bin/bash
# Posts a sync-cron completion heartbeat to Healthchecks.io.
# Invoked from crontab after successful sync completion. The HC URL is looked
# up by env-var name so the URL itself never appears in the crontab line.
# Usage: ping-sync-completion.sh <ENV_VAR_NAME>
# Example: ping-sync-completion.sh HC_PING_SYNC_PROD
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=/dev/null
source /opt/algoedgefno/env/healthchecks.env
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/_hc-ping.sh"

_hc_cfg=""
cleanup() { [[ -n "${_hc_cfg}" ]] && rm -f "${_hc_cfg}"; }
trap cleanup EXIT

var_name="${1:?usage: ping-sync-completion.sh <ENV_VAR_NAME>}"
hc_ping "${!var_name:-}" ""
