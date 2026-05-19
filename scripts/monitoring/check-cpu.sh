#!/bin/bash
# Exits 0 if the 5-minute load average is below 90% of available CPU cores.
# Exits 1 with a one-line stderr message when load is high.
set -euo pipefail

loadavg_5min=$(awk '{print $2}' /proc/loadavg)
nproc=$(nproc)

# Use awk for the floating-point comparison (bash arithmetic is integer-only).
overloaded=$(awk -v load="${loadavg_5min}" -v cores="${nproc}" \
    'BEGIN { print (load >= cores * 0.9) ? "1" : "0" }')

if [[ "${overloaded}" == "1" ]]; then
    printf 'cpu: 5min load %s >= %s cores * 0.9\n' "${loadavg_5min}" "${nproc}" >&2
    exit 1
fi

exit 0
