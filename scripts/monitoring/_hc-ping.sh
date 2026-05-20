#!/bin/bash
# Sourced helper — not a standalone CLI script.
# Provides hc_ping() for sending success or fail pings to Healthchecks.io.
# The ping URL contains the HC UUID, which is a credential. This helper uses
# the curl --config temp-file pattern so the URL never appears in ps output
# or cron logs.
#
# Usage after sourcing:
#   hc_ping "$HC_PING_VPS_HEALTH" ""                  # success ping (empty body)
#   hc_ping "$HC_PING_VPS_HEALTH/fail" "$diag_body"   # fail ping with body

# _hc_cfg tracks the temp file so the EXIT trap in the consumer can clean it.
# Each consumer that sources this file must declare its own cleanup trap if it
# needs to ensure cleanup on unexpected exit.
_hc_cfg=""

# hc_ping sends a ping to the given Healthchecks.io URL.
# $1 — full HC ping URL (success or /fail variant).
# $2 — optional plain-text body for /fail pings; empty string for success.
# Transient HC outage does not abort the caller: failures are silently swallowed.
hc_ping() {
    local url="$1"
    local body="$2"

    [[ -z "${url}" ]] && return 0

    _hc_cfg=$(mktemp)
    chmod 600 "${_hc_cfg}"
    printf 'url = "%s"\n' "${url}" > "${_hc_cfg}"

    if [[ -n "${body}" ]]; then
        curl -fsS --max-time 10 --config "${_hc_cfg}" \
            --data-raw "${body}" > /dev/null 2>&1 || true
    else
        curl -fsS --max-time 10 --config "${_hc_cfg}" \
            > /dev/null 2>&1 || true
    fi

    rm -f "${_hc_cfg}"
    _hc_cfg=""
}
