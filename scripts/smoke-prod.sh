#!/bin/bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
    printf 'usage: %s <commit-sha> <migration-version>\n' "$0" >&2
    exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

SMOKE_BASE_URL="${SMOKE_BASE_URL:-https://api.algoedgefno.com}" \
EXPECTED_ENV=production \
EXPECTED_COMMIT="$1" \
EXPECTED_MIGRATION="$2" \
CONTAINER_NAME="${CONTAINER_NAME:-algoedgefno-backend-prod}" \
APP_ENV_FILE="${APP_ENV_FILE:-/opt/algoedgefno/env/prod.env}" \
"${script_dir}/smoke-deploy.sh"
