#!/bin/bash
# Exits 0 if memory usage is below 80%.
# Exits 1 with a one-line stderr message if usage is at or above the threshold.
set -euo pipefail

readonly THRESHOLD_PCT=80

read -r total used _ < <(free -m | awk 'NR==2 {print $2, $3, $0}')

pct=$(( used * 100 / total ))

if [[ "${pct}" -ge "${THRESHOLD_PCT}" ]]; then
    printf 'mem: %s%% used (%sMiB of %sMiB)\n' "${pct}" "${used}" "${total}" >&2
    exit 1
fi

exit 0
