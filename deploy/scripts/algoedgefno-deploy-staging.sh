#!/bin/bash
set -euo pipefail

PATH="/usr/sbin:/usr/bin:/sbin:/bin"

if [[ $# -ne 2 ]]; then
    printf 'usage: %s <digest-qualified-image> <staging-base-url>\n' "$0" >&2
    exit 2
fi

DEPLOY_IMAGE="$1"
STAGING_BASE_URL="${2%/}"
IMAGE_PATTERN='^ghcr\.io/deependra191/algoedgefno-backend@sha256:[0-9a-f]{64}$'
COMMIT_SHA_PATTERN='^[0-9a-f]{7,64}$'
MIGRATION_VERSION_PATTERN='^[0-9]+$'
COMPOSE_DIR="/opt/algoedgefno/compose"
ENV_FILE="${COMPOSE_DIR}/.env"
ENV_BACKUP_PREFIX="${ENV_FILE}.bak.preflight."
SMOKE_SCRIPT="/opt/algoedgefno/scripts/smoke-staging.sh"

fail() {
    printf 'FAIL %s\n' "$1" >&2
    exit 1
}

pass() {
    printf 'PASS %s\n' "$1"
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

# status_code returns non-zero on HTTP code mismatch so post-deploy callers can roll back.
status_code() {
    local name="$1"
    local want="$2"
    local url="$3"
    local code

    code="$(curl -sS -o /dev/null -w '%{http_code}' "${url}" || true)"
    if [[ "${code}" != "${want}" ]]; then
        printf 'FAIL %s: got HTTP %s, want %s\n' "${name}" "${code}" "${want}" >&2
        return 1
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

env_file_value() {
    local key="$1"
    local line

    line="$(grep -E "^${key}=" "${ENV_FILE}" | tail -1 || true)"
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

image_migration_version() {
    local image="$1"

    docker run --rm --entrypoint /bin/sh "${image}" -c '
set -eu
max=0
for file in /app/migrations/*.up.sql; do
    name="${file##*/}"
    version="${name%%_*}"
    version="${version#"${version%%[!0]*}"}"
    version="${version:-0}"
    if [ "${version}" -gt "${max}" ]; then
        max="${version}"
    fi
done
printf "%s\n" "${max}"
'
}

if [[ ! "${DEPLOY_IMAGE}" =~ ${IMAGE_PATTERN} ]]; then
    fail "image must be ghcr.io/deependra191/algoedgefno-backend@sha256:<64 lowercase hex chars>"
fi

if [[ ! "${STAGING_BASE_URL}" =~ ^https://(staging-api\.[A-Za-z0-9.-]+|staging-[A-Za-z0-9.-]+|[A-Za-z0-9.-]+\.staging\.[A-Za-z0-9.-]+)(:[0-9]{1,5})?$ ]]; then
    fail "staging base URL must be an https staging host without a path"
fi
staging_host="${STAGING_BASE_URL#https://}"
staging_host="${staging_host%%:*}"

require_cmd awk
require_cmd curl
require_cmd docker
require_cmd install
require_cmd mktemp
require_cmd python3

[[ -f "${ENV_FILE}" ]] || fail "${ENV_FILE} is missing"
[[ -x "${SMOKE_SCRIPT}" ]] || fail "${SMOKE_SCRIPT} must exist and be executable"
grep -q '^BACKEND_PROD_IMAGE=' "${ENV_FILE}" || fail "BACKEND_PROD_IMAGE must already be set; production image was not modified"

configured_staging_host="$(env_file_value STAGING_API_HOST || true)"
if [[ -z "${configured_staging_host}" ]]; then
    fail "STAGING_API_HOST must be set in ${ENV_FILE}"
fi
if [[ "${staging_host}" != "${configured_staging_host}" ]]; then
    fail "staging URL host ${staging_host} does not match configured STAGING_API_HOST ${configured_staging_host}"
fi

previous_staging_image="$(env_file_value BACKEND_STAGING_IMAGE || true)"

tmp_env="$(mktemp)"
version_body="$(mktemp)"
env_backup=""
cleanup() {
    rm -f "${tmp_env}" "${version_body}"
}
trap cleanup EXIT

restore_previous_staging_image() {
    install -o root -g root -m 600 "${env_backup}" "${ENV_FILE}"
    (
        cd "${COMPOSE_DIR}"
        docker compose --profile staging up -d backend-staging
    )
}

rollback_and_fail() {
    local reason="$1"
    if [[ -z "${previous_staging_image}" ]]; then
        fail "${reason} on ${DEPLOY_IMAGE}; no previous BACKEND_STAGING_IMAGE to restore"
    fi
    if ! restore_previous_staging_image; then
        fail "ROLLBACK FAILED after ${reason} on ${DEPLOY_IMAGE}; manual recovery required (env backup at ${env_backup})"
    fi
    fail "${reason} on ${DEPLOY_IMAGE}; restored previous BACKEND_STAGING_IMAGE ${previous_staging_image}"
}

docker pull "${DEPLOY_IMAGE}"
expected_migration="$(image_migration_version "${DEPLOY_IMAGE}")"
if [[ ! "${expected_migration}" =~ ${MIGRATION_VERSION_PATTERN} ]]; then
    fail "image migration version must be a non-negative integer, got ${expected_migration}"
fi
pass "image migration version: ${expected_migration}"

env_backup="$(mktemp "${ENV_BACKUP_PREFIX}XXXXXX")"
cp "${ENV_FILE}" "${env_backup}"
awk -v image="${DEPLOY_IMAGE}" '
    BEGIN { updated = 0 }
    /^BACKEND_STAGING_IMAGE=/ {
        print "BACKEND_STAGING_IMAGE=" image
        updated = 1
        next
    }
    { print }
    END {
        if (updated == 0) {
            print "BACKEND_STAGING_IMAGE=" image
        }
    }
' "${ENV_FILE}" > "${tmp_env}"
install -o root -g root -m 600 "${tmp_env}" "${ENV_FILE}"

cd "${COMPOSE_DIR}"
if ! docker compose --profile migrate-staging run --rm migrate-staging; then
    rollback_and_fail "failed to run staging migrations"
fi

if ! docker compose --profile staging up -d backend-staging; then
    rollback_and_fail "failed to restart backend-staging"
fi

if ! status_code health 200 "${STAGING_BASE_URL}/health"; then
    rollback_and_fail "staging health check failed"
fi
if ! status_code ready 200 "${STAGING_BASE_URL}/ready"; then
    rollback_and_fail "staging ready check failed"
fi

version_code="$(curl -fsS -o "${version_body}" -w '%{http_code}' "${STAGING_BASE_URL}/version" || true)"
if [[ "${version_code}" != "200" ]]; then
    rollback_and_fail "staging version returned HTTP ${version_code}, want 200"
fi

environment="$(json_field "${version_body}" environment)"
expected_commit="$(json_field "${version_body}" commit_sha)"
actual_migration="$(json_field "${version_body}" migration_version)"
if [[ "${environment}" != "staging" ]]; then
    rollback_and_fail "staging version environment was ${environment}, want staging"
fi
if [[ -z "${expected_commit}" ]]; then
    rollback_and_fail "staging version must report commit_sha"
fi
if [[ ! "${expected_commit}" =~ ${COMMIT_SHA_PATTERN} ]]; then
    rollback_and_fail "staging version commit_sha must be 7-64 lowercase hex chars"
fi
if [[ "${actual_migration}" != "${expected_migration}" ]]; then
    rollback_and_fail "staging version migration_version was ${actual_migration}, want ${expected_migration}"
fi
pass "version: commit ${expected_commit}, migration ${expected_migration}"

if ! EXPECTED_IMAGE="${DEPLOY_IMAGE}" \
    SMOKE_BASE_URL="${STAGING_BASE_URL}" \
        "${SMOKE_SCRIPT}" "${expected_commit}" "${expected_migration}"; then
    rollback_and_fail "staging smoke failed"
fi

printf 'staging deploy passed for %s at migration %s\n' "${DEPLOY_IMAGE}" "${expected_migration}"
