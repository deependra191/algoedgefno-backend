#!/bin/bash
# Exits 0 if disk usage on / and /opt is below 85%.
# Exits 1 with a one-line stderr message listing each failing mount.
set -euo pipefail

readonly THRESHOLD_PCT=85

check_mount() {
    local mount="$1"
    local pct
    pct=$(df -P "${mount}" | awk 'NR==2 {gsub(/%/,""); print $5}')
    if [[ "${pct}" -ge "${THRESHOLD_PCT}" ]]; then
        printf '%s: %s%% used on %s\n' "disk" "${pct}" "${mount}" >&2
        return 1
    fi
    return 0
}

root_source=$(df --output=source / | tail -1)
opt_source=$(df --output=source /opt | tail -1)

failed=0

check_mount "/" || failed=1

# Only check /opt separately when it is on a different device.
if [[ "${opt_source}" != "${root_source}" ]]; then
    check_mount "/opt" || failed=1
fi

exit "${failed}"
