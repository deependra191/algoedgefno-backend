#!/bin/bash
set -euo pipefail

BASE_URL="${SMOKE_BASE_URL:-}"
COMPOSE_DIR="${COMPOSE_DIR:-/opt/algoedgefno/compose}"
CONTAINER_NAME="${CONTAINER_NAME:-}"
EXPECTED_ENV="${EXPECTED_ENV:-}"
EXPECTED_MIGRATION="${EXPECTED_MIGRATION:-}"
EXPECTED_IMAGE="${EXPECTED_IMAGE:-}"
EXPECTED_COMMIT="${EXPECTED_COMMIT:-}"
APP_ENV_FILE="${APP_ENV_FILE:-}"
APP_TOKEN="${APP_TOKEN:-}"
DB_NAME="${DB_NAME:-}"
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

    local body="${tmpdir}/${name}.body"
    local code
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

load_app_env() {
    if [[ -n "${APP_TOKEN}" && -n "${DB_NAME}" ]]; then
        return
    fi
    if [[ -z "${APP_ENV_FILE}" ]]; then
        fail "APP_ENV_FILE must be set unless APP_TOKEN and DB_NAME are both set"
    fi
    if [[ ! -r "${APP_ENV_FILE}" ]]; then
        fail "app env values unset and ${APP_ENV_FILE} is not readable"
    fi

    if [[ -z "${APP_TOKEN}" ]]; then
        APP_TOKEN="$(env_file_value APP_SECRET_TOKEN "${APP_ENV_FILE}" || true)"
        if [[ -z "${APP_TOKEN}" ]]; then
            fail "APP_SECRET_TOKEN missing or empty in ${APP_ENV_FILE}"
        fi
    fi

    if [[ -z "${DB_NAME}" ]]; then
        DB_NAME="$(env_file_value DB_NAME "${APP_ENV_FILE}" || true)"
        if [[ -z "${DB_NAME}" ]]; then
            fail "DB_NAME missing or empty in ${APP_ENV_FILE}"
        fi
    fi
}

lower() {
    printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

contains_any() {
    local value
    value="$(lower "$1")"
    shift
    for marker in "$@"; do
        [[ "${value}" == *"${marker}"* ]] && return 0
    done
    return 1
}

reject_markers() {
    local label="$1"
    local value="$2"
    shift 2
    if contains_any "${value}" "$@"; then
        fail "${label} must not contain any of: $*"
    fi
}

require_markers() {
    local label="$1"
    local value="$2"
    shift 2
    if ! contains_any "${value}" "$@"; then
        fail "${label} must contain one of: $*"
    fi
}

validate_environment() {
    case "${EXPECTED_ENV}" in
        staging)
            require_markers SMOKE_BASE_URL "${BASE_URL}" staging stage
            require_markers CONTAINER_NAME "${CONTAINER_NAME}" staging stage
            require_markers DB_NAME "${DB_NAME}" staging stage
            reject_markers SMOKE_BASE_URL "${BASE_URL}" prod production
            reject_markers CONTAINER_NAME "${CONTAINER_NAME}" prod production
            reject_markers DB_NAME "${DB_NAME}" prod production
            ;;
        production)
            require_markers CONTAINER_NAME "${CONTAINER_NAME}" prod production
            require_markers DB_NAME "${DB_NAME}" prod production
            reject_markers SMOKE_BASE_URL "${BASE_URL}" staging stage
            reject_markers CONTAINER_NAME "${CONTAINER_NAME}" staging stage
            reject_markers DB_NAME "${DB_NAME}" staging stage
            ;;
        *)
            fail "EXPECTED_ENV must be staging or production"
            ;;
    esac
}

auth_config() {
    local cfg="${tmpdir}/curl-auth.conf"
    chmod 700 "${tmpdir}"
    printf 'header = "Authorization: Bearer %s"\n' "${APP_TOKEN}" > "${cfg}"
    chmod 600 "${cfg}"
    printf '%s' "${cfg}"
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

if [[ -z "${EXPECTED_COMMIT}" ]]; then
    fail "EXPECTED_COMMIT must be set to the immutable commit SHA"
fi
if [[ -z "${EXPECTED_MIGRATION}" ]]; then
    fail "EXPECTED_MIGRATION must be set to the expected clean migration version"
fi
if [[ -z "${EXPECTED_ENV}" ]]; then
    fail "EXPECTED_ENV must be set to staging or production"
fi
if [[ -z "${BASE_URL}" ]]; then
    fail "SMOKE_BASE_URL must be set"
fi
if [[ -z "${CONTAINER_NAME}" ]]; then
    fail "CONTAINER_NAME must be set"
fi
if [[ -z "${EXPECTED_IMAGE}" ]]; then
    EXPECTED_IMAGE="ghcr.io/deependra191/algoedgefno-backend:${EXPECTED_COMMIT}"
fi

load_app_env
validate_environment

status_code health 200 "${BASE_URL}/health"
status_code ready 200 "${BASE_URL}/ready"

version_body="${tmpdir}/version.body"
version_code="$(curl -fsS -o "${version_body}" -w '%{http_code}' "${BASE_URL}/version" || true)"
if [[ "${version_code}" != "200" ]]; then
    fail "version: got HTTP ${version_code}, want 200"
fi

commit_sha="$(json_field "${version_body}" commit_sha)"
environment="$(json_field "${version_body}" environment)"
migration_version="$(json_field "${version_body}" migration_version)"
if [[ "${commit_sha}" != "${EXPECTED_COMMIT}" ]]; then
    fail "version commit_sha: got ${commit_sha}, want ${EXPECTED_COMMIT}"
fi
if [[ "${environment}" != "${EXPECTED_ENV}" ]]; then
    fail "version environment: got ${environment}, want ${EXPECTED_ENV}"
fi
if [[ "${migration_version}" != "${EXPECTED_MIGRATION}" ]]; then
    fail "version migration_version: got ${migration_version}, want ${EXPECTED_MIGRATION}"
fi
pass "version: commit/environment/migration match"

status_code protected-no-token 401 "${BASE_URL}/api/v1/config/app"
status_code protected-bad-token 401 -H 'Authorization: Bearer bad-token' "${BASE_URL}/api/v1/config/app"

status_code protected-valid-token 200 --config "$(auth_config)" "${BASE_URL}/api/v1/config/app"

actual_image="$(docker inspect "${CONTAINER_NAME}" --format '{{.Config.Image}}')"
if [[ "${actual_image}" != "${EXPECTED_IMAGE}" ]]; then
    fail "container image: got ${actual_image}, want ${EXPECTED_IMAGE}"
fi
pass "container image: ${EXPECTED_IMAGE}"

migration_state="$(
    cd "${COMPOSE_DIR}" && docker compose exec -T postgres sh -c \
        'psql -U "$POSTGRES_USER" -d "$1" -tA -F "|" -c "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1;"' \
        sh "${DB_NAME}"
)"
if [[ "${migration_state}" != "${EXPECTED_MIGRATION}|f" ]]; then
    fail "schema_migrations: got ${migration_state}, want ${EXPECTED_MIGRATION}|f"
fi
pass "schema_migrations: version ${EXPECTED_MIGRATION}, dirty false"

headers="${tmpdir}/cors.headers"
curl -fsS -D "${headers}" -o /dev/null -H 'Origin: https://example.com' "${BASE_URL}/health"
if grep -Eiq '^access-control-' "${headers}"; then
    fail "CORS: unexpected access-control-* response header"
fi
pass "CORS: no access-control-* response headers"

logs="${tmpdir}/deploy.logs"
docker logs --since "${LOG_SINCE}" "${CONTAINER_NAME}" > "${logs}" 2>&1
if ! python3 - "${logs}" "${EXPECTED_COMMIT}" "${EXPECTED_ENV}" <<'PY'
import json
import sys

required = {"method", "path", "status", "latency_ms", "env", "version", "commit", "request_id"}
expected_commit = sys.argv[2]
expected_env = sys.argv[3]

with open(sys.argv[1], "r", encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError:
            continue
        if required.issubset(record) and record.get("commit") == expected_commit and record.get("env") == expected_env:
            sys.exit(0)
sys.exit(1)
PY
then
    fail "logs: no sampled request log had the expected shape"
fi

if grep -Eiq '(authorization|bearer |database_url|db_password|jwt_secret|app_secret_token|postgres://|password=|token=|secret=|dsn=)' "${logs}"; then
    fail "logs: sampled logs contain an obvious secret-like pattern"
fi
pass "logs: expected shape and no obvious secret leakage"

printf '%s smoke passed for %s\n' "${EXPECTED_ENV}" "${EXPECTED_COMMIT}"
