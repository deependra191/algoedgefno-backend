#!/bin/bash
set -euo pipefail

PATH="/usr/sbin:/usr/bin:/sbin:/bin"

if [[ $# -ne 2 ]]; then
    printf 'usage: %s <prod-base-url> <staging-base-url>\n' "$0" >&2
    exit 2
fi

PROD_BASE_URL="${1%/}"
STAGING_BASE_URL="${2%/}"
IMAGE_PATTERN='^ghcr\.io/deependra191/algoedgefno-backend@sha256:[0-9a-f]{64}$'
HTTPS_HOST_PATTERN='^https://[A-Za-z0-9.-]+(:[0-9]{1,5})?$'
COMPOSE_DIR="/opt/algoedgefno/compose"
ENV_FILE="${COMPOSE_DIR}/.env"
PROD_CONTAINER="algoedgefno-backend-prod"
STAGING_CONTAINER="algoedgefno-backend-staging"
SMOKE_SCRIPT="/opt/algoedgefno/scripts/smoke-prod.sh"
ENV_BACKUP_PREFIX="${ENV_FILE}.bak.preflight."

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
[[ -x "${SMOKE_SCRIPT}" ]] || fail "${SMOKE_SCRIPT} must exist and be executable"

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
env_backup="$(mktemp "${ENV_BACKUP_PREFIX}XXXXXX")"
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

actual_staging_image="$(docker inspect "${STAGING_CONTAINER}" --format '{{.Config.Image}}' 2>/dev/null || true)"
if [[ -z "${actual_staging_image}" ]]; then
    fail "staging container ${STAGING_CONTAINER} is not running"
fi
if [[ "${actual_staging_image}" != "${deploy_image}" ]]; then
    fail "staging container image ${actual_staging_image} does not match BACKEND_STAGING_IMAGE ${deploy_image}"
fi
pass "staging container image: ${deploy_image}"

status_code staging-health 200 "${STAGING_BASE_URL}/health"
status_code staging-ready 200 "${STAGING_BASE_URL}/ready"

version_code="$(curl -fsS -o "${version_body}" -w '%{http_code}' "${STAGING_BASE_URL}/version" || true)"
if [[ "${version_code}" != "200" ]]; then
    fail "staging version: got HTTP ${version_code}, want 200"
fi

expected_commit="$(json_field "${version_body}" commit_sha)"
expected_environment="$(json_field "${version_body}" environment)"
expected_migration="$(json_field "${version_body}" migration_version)"

if [[ "${expected_environment}" != "staging" ]]; then
    fail "staging version environment: got ${expected_environment}, want staging"
fi
if [[ -z "${expected_commit}" ]]; then
    fail "staging version must report commit_sha"
fi
if [[ -z "${expected_migration}" ]]; then
    fail "staging version must report migration_version"
fi
pass "staging version: commit ${expected_commit}, migration ${expected_migration}"

docker pull "${deploy_image}"

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
if ! docker compose up -d backend-prod; then
    restore_previous_prod_image
    fail "failed to restart backend-prod on ${deploy_image}; restored previous BACKEND_PROD_IMAGE ${previous_prod_image}"
fi

if ! EXPECTED_IMAGE="${deploy_image}" \
    SMOKE_BASE_URL="${PROD_BASE_URL}" \
        "${SMOKE_SCRIPT}" "${expected_commit}" "${expected_migration}"; then
    restore_previous_prod_image
    fail "production smoke failed on ${deploy_image}; restored previous BACKEND_PROD_IMAGE ${previous_prod_image}"
fi

actual_prod_image="$(docker inspect "${PROD_CONTAINER}" --format '{{.Config.Image}}')"
if [[ "${actual_prod_image}" != "${deploy_image}" ]]; then
    restore_previous_prod_image
    fail "prod container image: got ${actual_prod_image}, want ${deploy_image}; restored previous BACKEND_PROD_IMAGE ${previous_prod_image}"
fi
pass "prod container image: ${deploy_image}"

printf 'PROMOTED_IMAGE=%s\n' "${deploy_image}"
printf 'PROMOTED_COMMIT=%s\n' "${expected_commit}"
printf 'PROMOTED_MIGRATION=%s\n' "${expected_migration}"
printf 'PREVIOUS_PROD_IMAGE=%s\n' "${previous_prod_image}"
printf 'ENV_BACKUP=%s\n' "${env_backup}"
printf 'production deploy passed for %s\n' "${deploy_image}"
