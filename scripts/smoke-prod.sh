#!/bin/bash
# smoke-prod.sh — post-launch mutating production smoke.
# Requires PROD_SMOKE_UID (provisioned per §10 Step 9); performs
# firebase-token exchange → /auth/session → tenant assert → /auth/logout.
# Production-only: no staging UIDs, no fixture scripts.
set -euo pipefail

if [[ $# -ne 2 ]]; then
    printf 'usage: %s <commit-sha> <migration-version>\n' "$0" >&2
    exit 2
fi

EXPECTED_COMMIT="$1"
EXPECTED_MIGRATION="$2"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BASE_URL="${SMOKE_BASE_URL:-https://api.algoedgefno.com}"
CONTAINER_NAME="${CONTAINER_NAME:-algoedgefno-backend-prod}"
APP_ENV_FILE="${APP_ENV_FILE:-/opt/algoedgefno/env/prod.env}"
COMPOSE_DIR="${COMPOSE_DIR:-/opt/algoedgefno/compose}"
LOG_SINCE="${LOG_SINCE:-10m}"

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

env_file_value() {
    local key="$1"
    local file="$2"
    local line
    line="$(grep -E "^${key}=" "${file}" | tail -1 || true)"
    if [[ -z "${line}" ]]; then
        return 1
    fi
    local value="${line#*=}"
    value="${value%\"}"
    value="${value#\"}"
    value="${value%\'}"
    value="${value#\'}"
    printf '%s' "${value}"
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
require_cmd docker
require_cmd python3

if [[ ! -r "${APP_ENV_FILE}" ]]; then
    fail "app env file not readable: ${APP_ENV_FILE}"
fi

app_token="$(env_file_value APP_SECRET_TOKEN "${APP_ENV_FILE}" || true)"
if [[ -z "${app_token}" ]]; then
    fail "APP_SECRET_TOKEN missing or empty in ${APP_ENV_FILE}"
fi
prod_smoke_uid="$(env_file_value PROD_SMOKE_UID "${APP_ENV_FILE}" || true)"
if [[ -z "${prod_smoke_uid}" ]]; then
    fail "PROD_SMOKE_UID missing or empty in ${APP_ENV_FILE} — provision per §10 Step 9 before using smoke_mode=standard"
fi

chmod 700 "${tmpdir}"

# Write APP_SECRET_TOKEN to mode-600 temp file; never inline in curl args.
auth_cfg="${tmpdir}/curl-auth.conf"
printf 'header = "Authorization: Bearer %s"\n' "${app_token}" > "${auth_cfg}"
chmod 600 "${auth_cfg}"

# 1. Non-identity baseline checks (same as launch smoke, but here as a guard).
status_code health 200 "${BASE_URL}/health"
status_code ready 200 "${BASE_URL}/ready"

version_body="${tmpdir}/version.body"
version_code="$(curl -fsS -o "${version_body}" -w '%{http_code}' "${BASE_URL}/version" || true)"
if [[ "${version_code}" != "200" ]]; then
    fail "version: got HTTP ${version_code}, want 200"
fi
actual_commit="$(json_field "${version_body}" commit_sha)"
actual_migration="$(json_field "${version_body}" migration_version)"
if [[ "${actual_commit}" != "${EXPECTED_COMMIT}" ]]; then
    fail "version commit_sha: got ${actual_commit}, want ${EXPECTED_COMMIT}"
fi
if [[ "${actual_migration}" != "${EXPECTED_MIGRATION}" ]]; then
    fail "version migration_version: got ${actual_migration}, want ${EXPECTED_MIGRATION}"
fi
pass "version: commit and migration match"

status_code config-app-with-token 200 --config "${auth_cfg}" "${BASE_URL}/api/v1/config/app"
status_code backtests-static-token-rejected 401 --config "${auth_cfg}" "${BASE_URL}/api/v1/backtests"

# 2. Firebase token exchange — produces a real Firebase ID token for PROD_SMOKE_UID.
# Token written to mode-600 temp file; never echoed.
id_token_file="${tmpdir}/firebase-id-token"
docker compose -f "${COMPOSE_DIR}/docker-compose.yml" \
    exec -T backend-prod \
    sh -c "/app/firebase-token --uid=\"${prod_smoke_uid}\"" > "${id_token_file}"
chmod 600 "${id_token_file}"

id_token="$(cat "${id_token_file}")"
if [[ -z "${id_token}" ]]; then
    fail "firebase-token returned empty ID token for PROD_SMOKE_UID"
fi
pass "firebase-token: obtained ID token for PROD_SMOKE_UID"

# Write Firebase ID token to mode-600 curl config file.
firebase_cfg="${tmpdir}/curl-firebase.conf"
printf 'header = "Authorization: Bearer %s"\n' "${id_token}" > "${firebase_cfg}"
chmod 600 "${firebase_cfg}"

# 3. /auth/session exchange — creates or refreshes the users row.
session_body="${tmpdir}/session.body"
session_payload="${tmpdir}/session.json"
printf '{"firebaseIdToken":"%s"}' "${id_token}" > "${session_payload}"
chmod 600 "${session_payload}"

session_code="$(curl -sS -o "${session_body}" -w '%{http_code}' \
    -X POST \
    -H 'Content-Type: application/json' \
    --data-binary "@${session_payload}" \
    "${BASE_URL}/api/v1/auth/session" || true)"
if [[ "${session_code}" != "200" ]]; then
    fail "auth-session: got HTTP ${session_code}, want 200"
fi
pass "auth-session: HTTP 200"

access_token="$(json_field "${session_body}" accessToken)"
refresh_token_value="$(json_field "${session_body}" refreshToken)"
if [[ -z "${access_token}" ]]; then
    fail "auth-session: response missing accessToken"
fi
pass "auth-session: accessToken present"

# Write access token to mode-600 curl config file.
access_cfg="${tmpdir}/curl-access.conf"
printf 'header = "Authorization: Bearer %s"\n' "${access_token}" > "${access_cfg}"
chmod 600 "${access_cfg}"

# 4. Tenant assert — /backtests with access token returns 200 (not 401).
status_code backtests-tenant-access 200 --config "${access_cfg}" "${BASE_URL}/api/v1/backtests"

# 5. Logout — revoke the session.
if [[ -n "${refresh_token_value}" ]]; then
    logout_payload="${tmpdir}/logout.json"
    printf '{"refreshToken":"%s"}' "${refresh_token_value}" > "${logout_payload}"
    chmod 600 "${logout_payload}"

    logout_code="$(curl -sS -o /dev/null -w '%{http_code}' \
        -X POST \
        -H 'Content-Type: application/json' \
        --data-binary "@${logout_payload}" \
        "${BASE_URL}/api/v1/auth/logout" || true)"
    if [[ "${logout_code}" != "204" ]]; then
        fail "auth-logout: got HTTP ${logout_code}, want 204"
    fi
    pass "auth-logout: HTTP 204"
else
    pass "auth-logout: skipped (no refresh token in session response)"
fi

# 6. Log redaction check.
"${script_dir}/security/check-log-redaction.sh" --env prod --since "${LOG_SINCE}"

printf 'standard smoke passed for %s\n' "${EXPECTED_COMMIT}"
