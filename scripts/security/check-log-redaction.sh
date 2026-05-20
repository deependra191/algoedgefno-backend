#!/bin/bash
set -euo pipefail

readonly ENV_STAGING="staging"
readonly ENV_PROD="prod"
readonly DEFAULT_LOG_SINCE="10 minutes ago"
readonly DEFAULT_STAGING_CONTAINER="algoedgefno-backend-staging"
readonly DEFAULT_PROD_CONTAINER="algoedgefno-backend-prod"
readonly EXIT_USAGE=2
readonly EXIT_FAILURE=1

usage() {
    cat >&2 <<'EOF'
usage: scripts/security/check-log-redaction.sh --env staging|prod [--since <window>] [--secret-file <path>]

Checks recent backend logs for obvious credential leaks without printing matching
log lines. Uses docker logs first, then journalctl if docker logs is unavailable.
Optional secret-file lines use LABEL=VALUE format; values are checked literally
and never printed.
EOF
}

fail_usage() {
    printf 'ERROR %s\n' "$1" >&2
    usage
    exit "${EXIT_USAGE}"
}

fail() {
    printf 'FAIL %s\n' "$1" >&2
    exit "${EXIT_FAILURE}"
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

docker_since_value() {
    local value="$1"
    if [[ "${value}" =~ ^([0-9]+)[[:space:]]+minutes?[[:space:]]+ago$ ]]; then
        printf '%sm' "${BASH_REMATCH[1]}"
    elif [[ "${value}" =~ ^([0-9]+)[[:space:]]+hours?[[:space:]]+ago$ ]]; then
        printf '%sh' "${BASH_REMATCH[1]}"
    else
        printf '%s' "${value}"
    fi
}

env_name=""
since="${DEFAULT_LOG_SINCE}"
secret_file=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --env)
            [[ $# -ge 2 ]] || fail_usage "--env requires a value"
            env_name="$2"
            shift 2
            ;;
        --since)
            [[ $# -ge 2 ]] || fail_usage "--since requires a value"
            since="$2"
            shift 2
            ;;
        --secret-file)
            [[ $# -ge 2 ]] || fail_usage "--secret-file requires a value"
            secret_file="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            fail_usage "unknown argument: $1"
            ;;
    esac
done

case "${env_name}" in
    "${ENV_STAGING}")
        container_name="${ABUSE_STAGING_CONTAINER:-${DEFAULT_STAGING_CONTAINER}}"
        ;;
    "${ENV_PROD}")
        container_name="${ABUSE_PROD_CONTAINER:-${DEFAULT_PROD_CONTAINER}}"
        ;;
    "")
        fail_usage "--env is required"
        ;;
    *)
        fail_usage "--env must be staging or prod"
        ;;
esac

tmpdir="$(mktemp -d)"
cleanup() {
    rm -rf "${tmpdir}"
}
trap cleanup EXIT

logs_file="${tmpdir}/backend.logs"
log_source=""
docker_since="$(docker_since_value "${since}")"

if command -v docker >/dev/null 2>&1 && docker logs --since "${docker_since}" "${container_name}" > "${logs_file}" 2>/dev/null; then
    log_source="docker:${container_name}"
else
    require_cmd journalctl
    if journalctl --since "${since}" CONTAINER_NAME="${container_name}" > "${logs_file}" 2>/dev/null; then
        log_source="journalctl:CONTAINER_NAME=${container_name}"
    else
        fail "could not read backend container logs for ${container_name} via docker logs or container-scoped journalctl"
    fi
fi

failures=0

check_pattern() {
    local label="$1"
    local pattern="$2"
    local count
    count="$(grep -Eic "${pattern}" "${logs_file}" || true)"
    if [[ "${count}" != "0" ]]; then
        printf 'FAIL log-redaction: %s matched %s line(s)\n' "${label}" "${count}" >&2
        failures=$((failures + 1))
    fi
}

check_literal() {
    local label="$1"
    local value="$2"
    local count
    [[ -n "${value}" ]] || return
    count="$(grep -Fic -- "${value}" "${logs_file}" || true)"
    if [[ "${count}" != "0" ]]; then
        printf 'FAIL log-redaction: %s matched %s line(s)\n' "${label}" "${count}" >&2
        failures=$((failures + 1))
    fi
}

check_pattern "authorization bearer marker" 'Bearer[[:space:]]+'
check_pattern "JWT_SECRET marker" 'JWT_SECRET'
check_pattern "APP_SECRET_TOKEN marker" 'APP_SECRET_TOKEN'
check_pattern "password key marker" 'password='
check_pattern "postgres DSN marker" 'postgres://'
check_pattern "DATABASE_URL marker" 'database_url|DATABASE_URL'
check_pattern "DB password marker" 'db_password|DB_PASSWORD'
check_pattern "token assignment marker" 'token='
check_pattern "secret assignment marker" 'secret='
check_pattern "DSN assignment marker" 'dsn='

if [[ -n "${secret_file}" ]]; then
    [[ -r "${secret_file}" ]] || fail "secret file is not readable"
    while IFS='=' read -r secret_label secret_value; do
        [[ -n "${secret_label}" ]] || continue
        check_literal "secret value: ${secret_label}" "${secret_value}"
    done < "${secret_file}"
fi

if [[ "${failures}" -ne 0 ]]; then
    fail "log redaction found ${failures} secret-like pattern(s) in ${log_source}"
fi

printf 'PASS log-redaction: no secret-like patterns in %s since %s\n' "${log_source}" "${since}"
