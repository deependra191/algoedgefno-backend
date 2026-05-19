#!/bin/bash
# Exits 0 if memory pressure is below the threshold.
# Exits 1 with a one-line stderr message if pressure is at or above it.
#
# "Pressure" = (total - available) / total. The kernel's MemAvailable column
# (free -m column 7) is the right denominator on Linux: it excludes
# reclaimable page cache, which the raw "used" column double-counts. Using
# raw used would fire false positives on any active server.
set -euo pipefail

readonly THRESHOLD_PCT=80

read -r total available < <(free -m | awk '$1=="Mem:" {print $2, $7}')

if [[ -z "${total:-}" || -z "${available:-}" || "${total}" -eq 0 ]]; then
    printf 'could not read memory from free -m\n' >&2
    exit 1
fi

pct=$(( (total - available) * 100 / total ))

if [[ "${pct}" -ge "${THRESHOLD_PCT}" ]]; then
    printf '%s%% used (%sMiB available of %sMiB total)\n' \
        "${pct}" "${available}" "${total}" >&2
    exit 1
fi

exit 0
