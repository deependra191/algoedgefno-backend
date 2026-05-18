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
COMPOSE_DIR="/opt/algoedgefno/compose"
ENV_FILE="${COMPOSE_DIR}/.env"
ENV_BACKUP_PREFIX="${ENV_FILE}.bak.preflight."
STAGING_CONTAINER="algoedgefno-backend-staging"

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
if [[ "${environment}" != "staging" ]]; then
    rollback_and_fail "staging version environment was ${environment}, want staging"
fi
pass "version: environment staging"

if ! status_code protected-no-token 401 "${STAGING_BASE_URL}/api/v1/config/app"; then
    rollback_and_fail "staging protected-no-token check failed"
fi

actual_image="$(docker inspect "${STAGING_CONTAINER}" --format '{{.Config.Image}}')"
if [[ "${actual_image}" != "${DEPLOY_IMAGE}" ]]; then
    rollback_and_fail "staging container image mismatch (got ${actual_image}, want ${DEPLOY_IMAGE})"
fi
pass "container image: ${DEPLOY_IMAGE}"

printf 'staging deploy passed for %s\n' "${DEPLOY_IMAGE}"
