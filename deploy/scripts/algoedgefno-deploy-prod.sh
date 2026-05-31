#!/bin/bash
set -euo pipefail

PATH="/usr/sbin:/usr/bin:/sbin:/bin"

SMOKE_MODE=""

# Parse flags before positional arguments.
while [[ $# -gt 0 ]]; do
    case "$1" in
        --smoke-mode=*)
            SMOKE_MODE="${1#--smoke-mode=}"
            shift
            ;;
        --smoke-mode)
            [[ $# -ge 2 ]] || { printf 'usage: %s [--smoke-mode=<launch|standard>] <prod-base-url> <staging-base-url>\n' "$0" >&2; exit 2; }
            SMOKE_MODE="$2"
            shift 2
            ;;
        --)
            shift
            break
            ;;
        -*)
            printf 'unknown flag: %s\n' "$1" >&2
            exit 2
            ;;
        *)
            break
            ;;
    esac
done

if [[ $# -ne 2 ]]; then
    printf 'usage: %s [--smoke-mode=<launch|standard>] <prod-base-url> <staging-base-url>\n' "$0" >&2
    exit 2
fi

PROD_BASE_URL="${1%/}"
STAGING_BASE_URL="${2%/}"
IMAGE_PATTERN='^ghcr\.io/deependra191/algoedgefno-backend@sha256:[0-9a-f]{64}$'
HTTPS_HOST_PATTERN='^https://[A-Za-z0-9.-]+(:[0-9]{1,5})?$'
COMMIT_SHA_PATTERN='^[0-9a-f]{7,64}$'
MIGRATION_VERSION_PATTERN='^[0-9]+$'
COMPOSE_DIR="/opt/algoedgefno/compose"
ENV_FILE="${COMPOSE_DIR}/.env"
PROD_CONTAINER="algoedgefno-backend-prod"
STAGING_CONTAINER="algoedgefno-backend-staging"
SMOKE_SCRIPT_LAUNCH="/opt/algoedgefno/scripts/smoke-prod-launch.sh"
SMOKE_SCRIPT_STANDARD="/opt/algoedgefno/scripts/smoke-prod.sh"
ENV_BACKUP_PREFIX="${ENV_FILE}.bak.preflight."
READINESS_TIMEOUT_SECONDS=60
READINESS_INTERVAL_SECONDS=1

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

status_code() {
    local name="$1"
    local want="$2"
    local url="$3"
    local code

    code="$(curl -sS -o /dev/null -w '%{http_code}' "${url}" || true)"
    if [[ "${code}" != "${want}" ]]; then
        fail "${name}: got HTTP ${code}, want ${want}"
    fi
    pass "${name}: HTTP ${want}"
}

# wait_for_status polls url until it returns want, or READINESS_TIMEOUT_SECONDS
# elapses. It tolerates the brief window where a freshly restarted container is
# "Started" but not yet listening (the reverse proxy returns 502 during init).
# Returns non-zero on timeout so callers can fail or roll back.
wait_for_status() {
    local name="$1"
    local want="$2"
    local url="$3"
    local code=""
    local elapsed=0

    while true; do
        code="$(curl -sS -o /dev/null -w '%{http_code}' "${url}" || true)"
        if [[ "${code}" == "${want}" ]]; then
            pass "${name}: HTTP ${want}"
            return 0
        fi
        if [[ "${elapsed}" -ge "${READINESS_TIMEOUT_SECONDS}" ]]; then
            printf 'FAIL %s: got HTTP %s, want %s after %ss\n' "${name}" "${code}" "${want}" "${READINESS_TIMEOUT_SECONDS}" >&2
            return 1
        fi
        sleep "${READINESS_INTERVAL_SECONDS}"
        elapsed=$((elapsed + READINESS_INTERVAL_SECONDS))
    done
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

if [[ ! "${PROD_BASE_URL}" =~ ${HTTPS_HOST_PATTERN} ]]; then
    fail "prod base URL must be an https host without a path"
fi
if [[ ! "${STAGING_BASE_URL}" =~ ${HTTPS_HOST_PATTERN} ]]; then
    fail "staging base URL must be an https host without a path"
fi

prod_host="${PROD_BASE_URL#https://}"
prod_host="${prod_host%%:*}"
staging_host="${STAGING_BASE_URL#https://}"
staging_host="${staging_host%%:*}"
if [[ "${prod_host}" == "${staging_host}" ]]; then
    fail "prod and staging hosts must differ"
fi

require_cmd awk
require_cmd curl
require_cmd docker
require_cmd install
require_cmd mktemp
require_cmd python3

[[ -f "${ENV_FILE}" ]] || fail "${ENV_FILE} is missing"
[[ -x "${SMOKE_SCRIPT_LAUNCH}" ]] || fail "${SMOKE_SCRIPT_LAUNCH} must exist and be executable"
[[ -x "${SMOKE_SCRIPT_STANDARD}" ]] || fail "${SMOKE_SCRIPT_STANDARD} must exist and be executable"

configured_prod_host="$(env_file_value PROD_API_HOST || true)"
configured_staging_host="$(env_file_value STAGING_API_HOST || true)"
deploy_image="$(env_file_value BACKEND_STAGING_IMAGE || true)"
previous_prod_image="$(env_file_value BACKEND_PROD_IMAGE || true)"

if [[ -z "${configured_prod_host}" ]]; then
    fail "PROD_API_HOST must be set in ${ENV_FILE}"
fi
if [[ -z "${configured_staging_host}" ]]; then
    fail "STAGING_API_HOST must be set in ${ENV_FILE}"
fi
if [[ "${prod_host}" != "${configured_prod_host}" ]]; then
    fail "prod URL host ${prod_host} does not match configured PROD_API_HOST ${configured_prod_host}"
fi
if [[ "${staging_host}" != "${configured_staging_host}" ]]; then
    fail "staging URL host ${staging_host} does not match configured STAGING_API_HOST ${configured_staging_host}"
fi
if [[ -z "${deploy_image}" ]]; then
    fail "BACKEND_STAGING_IMAGE must be set in ${ENV_FILE}"
fi
if [[ ! "${deploy_image}" =~ ${IMAGE_PATTERN} ]]; then
    fail "BACKEND_STAGING_IMAGE must be ghcr.io/deependra191/algoedgefno-backend@sha256:<64 lowercase hex chars>"
fi
if [[ -z "${previous_prod_image}" ]]; then
    fail "BACKEND_PROD_IMAGE must already be set in ${ENV_FILE}"
fi
if [[ ! "${previous_prod_image}" =~ ${IMAGE_PATTERN} ]]; then
    fail "BACKEND_PROD_IMAGE must be ghcr.io/deependra191/algoedgefno-backend@sha256:<64 lowercase hex chars>"
fi

tmp_env="$(mktemp)"
version_body="$(mktemp)"
env_backup=""
migration_applied=false
cleanup() {
    rm -f "${tmp_env}" "${version_body}"
}
trap cleanup EXIT

restore_previous_prod_image() {
    install -o root -g root -m 600 "${env_backup}" "${ENV_FILE}"
    (
        cd "${COMPOSE_DIR}"
        docker compose up -d backend-prod
    )
}

rollback_and_fail() {
    local reason="$1"
    local migration_note=""
    if [[ "${migration_applied}" == "true" ]]; then
        migration_note="; database migrations were applied and are not rolled back automatically"
    fi
    if ! restore_previous_prod_image; then
        fail "ROLLBACK FAILED after ${reason} on ${deploy_image}${migration_note}; manual recovery required (env backup at ${env_backup})"
    fi
    fail "${reason} on ${deploy_image}${migration_note}; restored previous BACKEND_PROD_IMAGE ${previous_prod_image}"
}

actual_staging_image="$(docker inspect "${STAGING_CONTAINER}" --format '{{.Config.Image}}' 2>/dev/null || true)"
if [[ -z "${actual_staging_image}" ]]; then
    fail "staging container ${STAGING_CONTAINER} is not running"
fi
if [[ "${actual_staging_image}" != "${deploy_image}" ]]; then
    fail "staging container image ${actual_staging_image} does not match BACKEND_STAGING_IMAGE ${deploy_image}"
fi
pass "staging container image: ${deploy_image}"

docker pull "${deploy_image}"
expected_image_migration="$(image_migration_version "${deploy_image}")"
if [[ ! "${expected_image_migration}" =~ ${MIGRATION_VERSION_PATTERN} ]]; then
    fail "image migration version must be a non-negative integer, got ${expected_image_migration}"
fi
pass "image migration version: ${expected_image_migration}"

wait_for_status staging-health 200 "${STAGING_BASE_URL}/health" || fail "staging-health: not ready within ${READINESS_TIMEOUT_SECONDS}s"
wait_for_status staging-ready 200 "${STAGING_BASE_URL}/ready" || fail "staging-ready: not ready within ${READINESS_TIMEOUT_SECONDS}s"

version_code="$(curl -fsS -o "${version_body}" -w '%{http_code}' "${STAGING_BASE_URL}/version" || true)"
if [[ "${version_code}" != "200" ]]; then
    fail "staging version: got HTTP ${version_code}, want 200"
fi

expected_commit="$(json_field "${version_body}" commit_sha)"
expected_environment="$(json_field "${version_body}" environment)"
staging_migration="$(json_field "${version_body}" migration_version)"

if [[ "${expected_environment}" != "staging" ]]; then
    fail "staging version environment: got ${expected_environment}, want staging"
fi
if [[ -z "${expected_commit}" ]]; then
    fail "staging version must report commit_sha"
fi
if [[ ! "${expected_commit}" =~ ${COMMIT_SHA_PATTERN} ]]; then
    fail "staging version commit_sha must be 7-64 lowercase hex chars"
fi
if [[ -z "${staging_migration}" ]]; then
    fail "staging version must report migration_version"
fi
if [[ ! "${staging_migration}" =~ ${MIGRATION_VERSION_PATTERN} ]]; then
    fail "staging version migration_version must be a non-negative integer"
fi
if [[ "${staging_migration}" != "${expected_image_migration}" ]]; then
    fail "staging version migration_version ${staging_migration} does not match image migration ${expected_image_migration}"
fi
expected_migration="${expected_image_migration}"
pass "staging version: commit ${expected_commit}, migration ${expected_migration}"

env_backup="$(mktemp "${ENV_BACKUP_PREFIX}XXXXXX")"
cp "${ENV_FILE}" "${env_backup}"
awk -v image="${deploy_image}" '
    BEGIN { updated = 0 }
    /^BACKEND_PROD_IMAGE=/ {
        print "BACKEND_PROD_IMAGE=" image
        updated = 1
        next
    }
    { print }
    END {
        if (updated == 0) {
            print "BACKEND_PROD_IMAGE=" image
        }
    }
' "${ENV_FILE}" > "${tmp_env}"
install -o root -g root -m 600 "${tmp_env}" "${ENV_FILE}"

cd "${COMPOSE_DIR}"
if ! docker compose --profile migrate-prod run --rm migrate-prod; then
    rollback_and_fail "failed to run production migrations"
fi
migration_applied=true

if ! docker compose up -d backend-prod; then
    rollback_and_fail "failed to restart backend-prod"
fi

# Wait for the freshly restarted prod service to become ready before smoke runs.
# The reverse proxy returns 502 during the brief container init window, and the
# smoke script's first /health check is a single immediate curl that would lose
# that race (mirrors the staging deploy's post-restart readiness gate).
if ! wait_for_status prod-health 200 "${PROD_BASE_URL}/health"; then
    rollback_and_fail "production health check failed"
fi
if ! wait_for_status prod-ready 200 "${PROD_BASE_URL}/ready"; then
    rollback_and_fail "production ready check failed"
fi

run_smoke() {
    case "${SMOKE_MODE:-}" in
        launch)
            EXPECTED_IMAGE="${deploy_image}" \
            SMOKE_BASE_URL="${PROD_BASE_URL}" \
                "${SMOKE_SCRIPT_LAUNCH}" "${expected_commit}" "${expected_migration}"
            ;;
        standard)
            EXPECTED_IMAGE="${deploy_image}" \
            SMOKE_BASE_URL="${PROD_BASE_URL}" \
                "${SMOKE_SCRIPT_STANDARD}" "${expected_commit}" "${expected_migration}"
            ;;
        *)
            fail "unknown smoke_mode: ${SMOKE_MODE}"
            ;;
    esac
}

if ! run_smoke; then
    rollback_and_fail "production smoke failed"
fi

actual_prod_image="$(docker inspect "${PROD_CONTAINER}" --format '{{.Config.Image}}')"
if [[ "${actual_prod_image}" != "${deploy_image}" ]]; then
    rollback_and_fail "prod container image mismatch (got ${actual_prod_image}, want ${deploy_image})"
fi
pass "prod container image: ${deploy_image}"

printf 'PROMOTED_IMAGE=%s\n' "${deploy_image}"
printf 'PROMOTED_COMMIT=%s\n' "${expected_commit}"
printf 'PROMOTED_MIGRATION=%s\n' "${expected_migration}"
printf 'PREVIOUS_PROD_IMAGE=%s\n' "${previous_prod_image}"
printf 'production deploy passed for %s\n' "${deploy_image}"
