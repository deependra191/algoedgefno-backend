#!/bin/bash
# Meta-check: runs every 5 minutes via cron.
# Runs each subsystem check in sequence. Collects all failures before
# pinging HC so a single /fail call carries the full diagnostic.
set -euo pipefail

# shellcheck source=/dev/null
source /opt/algoedgefno/env/healthchecks.env
: "${HC_PING_VPS_HEALTH:?healthchecks.env must set HC_PING_VPS_HEALTH}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=/dev/null
source "${SCRIPT_DIR}/_hc-ping.sh"

_hc_cfg=""
cleanup() { [[ -n "${_hc_cfg}" ]] && rm -f "${_hc_cfg}"; }
trap cleanup EXIT

# Subsystem identifiers — the strings that appear in Telegram alerts.
# Named constants so a reader does not need to chase down what each string means.
readonly SUBSYS_DISK="disk"
readonly SUBSYS_CERT_PROD="cert_prod"
readonly SUBSYS_CERT_STAGING="cert_staging"
readonly SUBSYS_MEM="mem"
readonly SUBSYS_CPU="cpu"
readonly SUBSYS_CONTAINERS="containers"
readonly SUBSYS_BACKUP="backup"

readonly HOST_PROD="api.algoedgefno.com"
readonly HOST_STAGING="staging-api.algoedgefno.com"

failures=()

run_check() {
    local subsystem="$1"
    shift
    local err
    if ! err=$("$@" 2>&1); then
        failures+=("${subsystem}: ${err}")
    fi
}

run_check "${SUBSYS_DISK}"         "${SCRIPT_DIR}/check-disk.sh"
run_check "${SUBSYS_CERT_PROD}"    "${SCRIPT_DIR}/check-cert.sh" "${HOST_PROD}"
run_check "${SUBSYS_CERT_STAGING}" "${SCRIPT_DIR}/check-cert.sh" "${HOST_STAGING}"
run_check "${SUBSYS_MEM}"          "${SCRIPT_DIR}/check-mem.sh"
run_check "${SUBSYS_CPU}"          "${SCRIPT_DIR}/check-cpu.sh"
run_check "${SUBSYS_CONTAINERS}"   "${SCRIPT_DIR}/check-containers.sh"
run_check "${SUBSYS_BACKUP}"       "${SCRIPT_DIR}/check-backup.sh"

if [[ "${#failures[@]}" -gt 0 ]]; then
    body=$(printf '%s\n' "${failures[@]}")
    # Echo to stderr so manual runs surface the diagnostic on the terminal
    # and cron's stderr-redirect captures it in vps-health-cron.log.
    printf '%s\n' "${body}" >&2
    hc_ping "${HC_PING_VPS_HEALTH}/fail" "${body}"
    exit 1
fi

hc_ping "${HC_PING_VPS_HEALTH}" ""

