#!/bin/bash
# Exits 0 if a *.dump backup exists under /opt/algoedgefno/backups/ and is
# no older than 7 days.
# Exits 1 with a one-line stderr message identifying the failure mode.
# .meta sidecars and .bak.preflight.* env-file deploy artifacts are excluded.
set -euo pipefail

readonly BACKUP_DIR="/opt/algoedgefno/backups"
readonly MAX_AGE_DAYS=7

# find -printf is GNU-only; the VPS runs Linux. macOS BSD find does not support it.
newest=$(find "${BACKUP_DIR}" -maxdepth 1 -name '*.dump' \
    -printf '%T@ %p\n' 2>/dev/null | sort -nr | head -1)

if [[ -z "${newest}" ]]; then
    printf 'no_backups_exist\n' >&2
    exit 1
fi

mtime_epoch="${newest%% *}"
# Truncate sub-second precision before arithmetic.
mtime_epoch="${mtime_epoch%%.*}"

now_epoch=$(date +%s)
age_days=$(( (now_epoch - mtime_epoch) / 86400 ))

if [[ "${age_days}" -gt "${MAX_AGE_DAYS}" ]]; then
    iso_ts=$(date -d "@${mtime_epoch}" -u +"%Y-%m-%dT%H:%M:%SZ")
    printf 'backup_age_7d_exceeded: last backup at %s\n' "${iso_ts}" >&2
    exit 1
fi

exit 0
