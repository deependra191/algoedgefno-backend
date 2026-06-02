#!/bin/bash
# smoke-prod-launch.sh — non-identity smoke for the production launch deploy.
# Runs ONLY checks that do not require a Firebase ID token or a users row.
# Auth endpoints are NOT called here (a CI grep asserts this).
# Use smoke-prod.sh for post-launch mutating smoke (smoke_mode=standard).
set -euo pipefail

if [[ $# -ne 2 ]]; then
    printf 'usage: %s <commit-sha> <migration-version>\n' "$0" >&2
    exit 2
fi

EXPECTED_COMMIT="$1"
EXPECTED_MIGRATION="$2"

BASE_URL="${SMOKE_BASE_URL:-https://api.algoedgefno.com}"
CONTAINER_NAME="${CONTAINER_NAME:-algoedgefno-backend-prod}"
tmpdir="$(mktemp -d)"
cleanup() {
    rm -rf "${tmpdir}"
}
trap cleanup EXIT

pass() {
    printf 'PASS %s\n' "$1"
}

fail() {
    printf 'FAIL %s\n' "$1" >&2
    exit 1
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

status_code() {
    local name="$1"
    local want="$2"
    shift 2
    local body code
    body="${tmpdir}/${name}.body"
    code="$(curl -sS -o "${body}" -w '%{http_code}' "$@" || true)"
    if [[ "${code}" != "${want}" ]]; then
        fail "${name}: got HTTP ${code}, want ${want}"
    fi
    pass "${name}: HTTP ${want}"
}

json_field() {
    local file="$1"
    local field="$2"
    python3 - "$file" "$field" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)
value = data.get(sys.argv[2])
print("" if value is None else value)
PY
}

require_cmd curl
require_cmd python3

# 1. Health and ready checks.
status_code health 200 "${BASE_URL}/health"
status_code ready 200 "${BASE_URL}/ready"

# 2. Version must match candidate commit and migration.
version_body="${tmpdir}/version.body"
version_code="$(curl -fsS -o "${version_body}" -w '%{http_code}' "${BASE_URL}/version" || true)"
if [[ "${version_code}" != "200" ]]; then
    fail "version: got HTTP ${version_code}, want 200"
fi
actual_commit="$(json_field "${version_body}" commit_sha)"
actual_migration="$(json_field "${version_body}" migration_version)"
actual_env="$(json_field "${version_body}" environment)"
if [[ "${actual_commit}" != "${EXPECTED_COMMIT}" ]]; then
    fail "version commit_sha: got ${actual_commit}, want ${EXPECTED_COMMIT}"
fi
if [[ "${actual_migration}" != "${EXPECTED_MIGRATION}" ]]; then
    fail "version migration_version: got ${actual_migration}, want ${EXPECTED_MIGRATION}"
fi
if [[ "${actual_env}" != "production" ]]; then
    fail "version environment: got ${actual_env}, want production"
fi
pass "version: commit/migration/environment match"

# 3. /config/app is public app bootstrap config.
status_code config-app-public 200 "${BASE_URL}/api/v1/config/app"

# 4. /backtests remains a protected tenant endpoint.
status_code backtests-no-auth-rejected 401 "${BASE_URL}/api/v1/backtests"
status_code backtests-bad-token-rejected 401 -H 'Authorization: Bearer bad-token' "${BASE_URL}/api/v1/backtests"

printf 'launch smoke passed for %s\n' "${EXPECTED_COMMIT}"
