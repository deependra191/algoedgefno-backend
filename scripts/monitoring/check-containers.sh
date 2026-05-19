#!/bin/bash
# Checks that well-known backend containers are running and have not restarted
# since the last check.
# Exits 0 if all containers are healthy.
# Exits 1 with one stderr line per failing container.
set -euo pipefail

readonly STATE_DIR="/var/lib/algoedge-monitoring"

readonly CONTAINER_PROD="algoedgefno-backend-prod"
readonly CONTAINER_STAGING="algoedgefno-backend-staging"

readonly CONTAINERS=("${CONTAINER_PROD}" "${CONTAINER_STAGING}")

mkdir -p "${STATE_DIR}"

failed=0

for container in "${CONTAINERS[@]}"; do
    inspect_out=$(docker inspect --format='{{.State.Status}} {{.RestartCount}}' \
        "${container}" 2>/dev/null) || {
        printf 'container_%s: not found\n' "${container}" >&2
        failed=1
        continue
    }

    read -r status restart_count <<< "${inspect_out}"

    if [[ "${status}" != "running" ]]; then
        printf 'container_%s: status=%s\n' "${container}" "${status}" >&2
        failed=1
        continue
    fi

    state_file="${STATE_DIR}/${container}.restarts"
    prev_count=0
    if [[ -f "${state_file}" ]]; then
        prev_count=$(< "${state_file}")
    fi

    if [[ "${restart_count}" -gt "${prev_count}" ]]; then
        printf 'container_%s: restart_count increased to %s (was %s)\n' \
            "${container}" "${restart_count}" "${prev_count}" >&2
        failed=1
    fi

    printf '%s\n' "${restart_count}" > "${state_file}"
done

exit "${failed}"
