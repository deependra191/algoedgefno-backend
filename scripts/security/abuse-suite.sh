#!/bin/bash
set -euo pipefail

readonly ENV_STAGING="staging"
readonly ENV_PROD="prod"
readonly DEFAULT_STAGING_BASE_URL="https://staging-api.algoedgefno.com"
readonly DEFAULT_PROD_BASE_URL="https://api.algoedgefno.com"
readonly DEFAULT_STAGING_CONTAINER="algoedgefno-backend-staging"
readonly DEFAULT_PROD_CONTAINER="algoedgefno-backend-prod"
# Compose SERVICE names (not container names) — `docker compose exec` addresses
# services. backend-staging has no explicit container_name in the compose file.
readonly DEFAULT_STAGING_SERVICE="backend-staging"
readonly DEFAULT_PROD_SERVICE="backend-prod"
readonly REPORT_DIR="scratch/security-runs"
readonly COMPOSE_DIR="${COMPOSE_DIR:-/opt/algoedgefno/compose}"

readonly PROTECTED_CONFIG_PATH="/api/v1/config/app"
readonly BACKTESTS_PATH="/api/v1/backtests"
readonly AUTH_SESSION_PATH="/api/v1/auth/session"
readonly AUTH_LOGOUT_PATH="/api/v1/auth/logout"
readonly STAGING_ONLY_SEED_SCRIPT="/opt/algoedgefno/scripts/staging-only/seed-conflict-fixture.sh"

readonly BURST_REQUESTS=50
readonly EXIT_USAGE=2
readonly EXIT_FAILURE=1

readonly HTTP_STATUS_OK=200
readonly HTTP_STATUS_ACCEPTED=202
readonly HTTP_STATUS_BAD_REQUEST=400
readonly HTTP_STATUS_UNAUTHORIZED=401
readonly HTTP_STATUS_FORBIDDEN=403
readonly HTTP_STATUS_NOT_FOUND=404
readonly HTTP_STATUS_CONFLICT=409
readonly HTTP_STATUS_UNPROCESSABLE_ENTITY=422
readonly HTTP_STATUS_TOO_MANY_REQUESTS=429

readonly ERR_MISSING_AUTH="missing or invalid authorization header"
readonly ERR_AUTH_NOT_ALLOWED="auth_not_allowed"
readonly ERR_IDENTITY_CONFLICT="identity_conflict"
readonly ERR_NO_CANDLE_DATA="no candle data available"
readonly ERR_NO_INSTRUMENT="no instrument found for underlying"
readonly ERR_CANDLE_COUNT_EXCEEDED="candle count exceeds maximum allowed"

usage() {
    cat >&2 <<'EOF'
usage: scripts/security/abuse-suite.sh --env staging|prod

Runs deterministic HTTP abuse checks and writes a sanitized markdown report
under scratch/security-runs/YYYY-MM-DD-{env}.md.

Staging path (ordered):
  1. static-token boundary (static token rejected 401 on tenant endpoints;
     true cross-user isolation is covered by tenant_isolation_test.go, not here)
  2. allowlist-denied (TEST_UID_DENIED session rejected 403 auth_not_allowed)
  3. email-conflict (TEST_UID_CONFLICT triggers identity_conflict 409)
  4. rate-limit burst (LAST — may trigger 429 for subsequent calls)

Production path (read-only — no /auth/* calls, no mutation):
  1. public health/version checks
  2. static-token /config/app 200
  3. static-token tenant-endpoint 401
  4. unauthenticated tenant-endpoint 401
  5. log redaction check

The suite never reads server env files and never prints bearer token values.

Environment variables:
  STAGING_APP_TOKEN   (required for staging)
  PROD_APP_TOKEN      (required for prod)
  TEST_UID_A          (required for staging; provisioned by setup-firebase-test-users)
  TEST_UID_DENIED     (required for staging)
  TEST_UID_CONFLICT   (required for staging)
EOF
}

fail_usage() {
    printf 'ERROR %s\n' "$1" >&2
    usage
    exit "${EXIT_USAGE}"
}

fail_fast() {
    printf 'FAIL %s\n' "$1" >&2
    exit "${EXIT_FAILURE}"
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || fail_fast "missing required command: $1"
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"

env_name=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --env)
            [[ $# -ge 2 ]] || fail_usage "--env requires a value"
            env_name="$2"
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
        base_url="${ABUSE_STAGING_BASE_URL:-${DEFAULT_STAGING_BASE_URL}}"
        container_name="${ABUSE_STAGING_CONTAINER:-${DEFAULT_STAGING_CONTAINER}}"
        service_name="${ABUSE_STAGING_SERVICE:-${DEFAULT_STAGING_SERVICE}}"
        token_var_name="STAGING_APP_TOKEN"
        app_token="${STAGING_APP_TOKEN:-}"
        ;;
    "${ENV_PROD}")
        base_url="${ABUSE_PROD_BASE_URL:-${DEFAULT_PROD_BASE_URL}}"
        container_name="${ABUSE_PROD_CONTAINER:-${DEFAULT_PROD_CONTAINER}}"
        service_name="${ABUSE_PROD_SERVICE:-${DEFAULT_PROD_SERVICE}}"
        token_var_name="PROD_APP_TOKEN"
        app_token="${PROD_APP_TOKEN:-}"
        ;;
    "")
        fail_usage "--env is required"
        ;;
    *)
        fail_usage "--env must be staging or prod"
        ;;
esac

if [[ -z "${app_token}" ]]; then
    fail_fast "${token_var_name} must be set in the shell"
fi

require_cmd curl
require_cmd python3

tmpdir="$(mktemp -d)"
auth_configs=()
secret_files=()
report_lock=""
fixture_seeded=false

cleanup() {
    local cfg
    for cfg in "${auth_configs[@]}"; do
        [[ -n "${cfg}" ]] && rm -f "${cfg}"
    done
    for cfg in "${secret_files[@]}"; do
        [[ -n "${cfg}" ]] && rm -f "${cfg}"
    done
    [[ -n "${report_lock}" && -d "${report_lock}" ]] && rmdir "${report_lock}" 2>/dev/null || true
    if [[ "${fixture_seeded}" == "true" ]]; then
        "${STAGING_ONLY_SEED_SCRIPT}" --teardown 2>/dev/null || true
    fi
    rm -rf "${tmpdir}"
}
trap cleanup EXIT

mkdir -p "${repo_root}/${REPORT_DIR}"
run_date="$(date -u +"%Y-%m-%d")"
run_started_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
report_path="${repo_root}/${REPORT_DIR}/${run_date}-${env_name}.md"
report_lock="${report_path}.lock"
if ! mkdir "${report_lock}" 2>/dev/null; then
    fail_fast "another abuse-suite run is writing ${report_path}"
fi
report_tmp="${tmpdir}/report.md"

{
    printf '# Security Abuse Suite Report\n\n'
    printf -- "- Started: \`%s\`\n" "${run_started_at}"
    printf -- "- Environment: \`%s\`\n" "${env_name}"
    printf -- "- Base URL: \`%s\`\n" "${base_url}"
    printf -- "- Container: \`%s\`\n\n" "${container_name}"
    printf '| Status | Test | Detail |\n'
    printf '|---|---|---|\n'
} > "${report_tmp}"

failures=0

record_result() {
    local status="$1"
    local name="$2"
    local detail="$3"
    local escaped_detail="${detail//$'\n'/ }"
    escaped_detail="${escaped_detail//|/\\/}"
    printf '| %s | %s | %s |\n' "${status}" "${name}" "${escaped_detail}" >> "${report_tmp}"
    printf '%s %s: %s\n' "${status}" "${name}" "${detail}"
    if [[ "${status}" == "FAIL" ]]; then
        failures=$((failures + 1))
    fi
}

auth_config() {
    local token="$1"
    local name="$2"
    local cfg="${tmpdir}/curl-auth-${name}.conf"
    chmod 700 "${tmpdir}"
    printf 'header = "Authorization: Bearer %s"\n' "${token}" > "${cfg}"
    chmod 600 "${cfg}"
    auth_configs+=("${cfg}")
    printf '%s' "${cfg}"
}

http_request() {
    local name="$1"
    shift
    local body="${tmpdir}/${name}.body"
    local code
    code="$(curl -sS -o "${body}" -w '%{http_code}' --max-time 30 "$@" || true)"
    printf '%s|%s' "${code}" "${body}"
}

json_error_value() {
    local body="$1"
    python3 - "$body" <<'PY'
import json
import sys

try:
    with open(sys.argv[1], "r", encoding="utf-8") as f:
        data = json.load(f)
except Exception:
    print("")
    raise SystemExit(0)

value = data.get("error")
print("" if value is None else value)
PY
}

assert_status() {
    local test_name="$1"
    local expected_status="$2"
    shift 2
    local result code body
    result="$(http_request "${test_name}" "$@")"
    code="${result%%|*}"
    body="${result#*|}"
    if [[ "${code}" == "${expected_status}" ]]; then
        record_result "PASS" "${test_name}" "HTTP ${expected_status}"
    else
        record_result "FAIL" "${test_name}" "got HTTP ${code}, want ${expected_status}"
    fi
}

assert_error_response() {
    local test_name="$1"
    local expected_status="$2"
    local expected_error="$3"
    shift 3
    local result code body actual_error
    result="$(http_request "${test_name}" "$@")"
    code="${result%%|*}"
    body="${result#*|}"
    actual_error="$(json_error_value "${body}")"
    if [[ "${code}" == "${expected_status}" && "${actual_error}" == "${expected_error}" ]]; then
        record_result "PASS" "${test_name}" "HTTP ${expected_status} with expected error JSON"
    else
        record_result "FAIL" "${test_name}" "got HTTP ${code} error '${actual_error}', want HTTP ${expected_status} '${expected_error}'"
    fi
}

selected_auth_cfg="$(auth_config "${app_token}" "selected")"
invalid_auth_cfg="$(auth_config "invalid-abuse-suite-token" "invalid")"

run_auth_checks() {
    assert_error_response "protected-no-auth" "${HTTP_STATUS_UNAUTHORIZED}" "${ERR_MISSING_AUTH}" \
        "${base_url}${PROTECTED_CONFIG_PATH}"

    assert_error_response "protected-invalid-token" "${HTTP_STATUS_UNAUTHORIZED}" "${ERR_MISSING_AUTH}" \
        --config "${invalid_auth_cfg}" \
        "${base_url}${PROTECTED_CONFIG_PATH}"

    assert_status "protected-valid-token" "${HTTP_STATUS_OK}" \
        --config "${selected_auth_cfg}" \
        "${base_url}${PROTECTED_CONFIG_PATH}"
}

# run_pr1_closed_interval_check: static token rejected (401) on tenant endpoints.
run_pr1_closed_interval_check() {
    local payload_file="${tmpdir}/pr1-closed-interval-post.json"
    printf '{}' > "${payload_file}"

    assert_status "pr1-static-token-get-backtests" "${HTTP_STATUS_UNAUTHORIZED}" \
        --config "${selected_auth_cfg}" \
        "${base_url}${BACKTESTS_PATH}"

    assert_status "pr1-static-token-post-backtests" "${HTTP_STATUS_UNAUTHORIZED}" \
        -X POST \
        -H 'Content-Type: application/json' \
        --data-binary "@${payload_file}" \
        --config "${selected_auth_cfg}" \
        "${base_url}${BACKTESTS_PATH}"
}

run_unauthenticated_tenant_check() {
    assert_status "unauthenticated-backtests" "${HTTP_STATUS_UNAUTHORIZED}" \
        "${base_url}${BACKTESTS_PATH}"
}

run_log_redaction_check() {
    local output
    local secret_file="${tmpdir}/log-redaction-secrets.txt"
    chmod 700 "${tmpdir}"
    printf 'target app token=%s\n' "${app_token}" > "${secret_file}"
    if [[ "${env_name}" == "${ENV_PROD}" && -n "${STAGING_APP_TOKEN:-}" ]]; then
        printf 'staging app token=%s\n' "${STAGING_APP_TOKEN}" >> "${secret_file}"
    fi
    chmod 600 "${secret_file}"
    secret_files+=("${secret_file}")

    if output="$("${script_dir}/check-log-redaction.sh" --env "${env_name}" --since "${run_started_at}" --secret-file "${secret_file}" 2>&1)"; then
        record_result "PASS" "log-redaction" "${output}"
    else
        record_result "FAIL" "log-redaction" "${output}"
    fi
}

# --- Staging-specific checks ---

firebase_id_token_for_uid() {
    local uid="$1"
    local token_file="${tmpdir}/firebase-token-${uid//[^a-zA-Z0-9_-]/}.txt"
    docker compose -f "${COMPOSE_DIR}/docker-compose.yml" \
        exec -T "${service_name}" \
        sh -c "/app/firebase-token --uid=\"${uid}\"" > "${token_file}"
    chmod 600 "${token_file}"
    cat "${token_file}"
}

run_cross_tenant_check() {
    # This live check verifies only the static-token boundary: the static
    # APP_SECRET_TOKEN must be rejected (401) on all tenant endpoints. True
    # cross-user isolation (user A cannot read user B's resources) is proven by
    # internal/handlers/tenant_isolation_test.go against real owned resources;
    # it is deliberately NOT exercised here to avoid creating/cleaning tenant
    # data on the live shared VPS.
    run_pr1_closed_interval_check
}

run_allowlist_denied_check() {
    local uid_denied="${TEST_UID_DENIED:-}"
    if [[ -z "${uid_denied}" ]]; then
        record_result "SKIP" "allowlist-denied-session" "TEST_UID_DENIED not set in shell"
        return
    fi

    local id_token
    if ! id_token="$(firebase_id_token_for_uid "${uid_denied}")"; then
        record_result "FAIL" "allowlist-denied-session" "firebase-token failed for TEST_UID_DENIED"
        failures=$((failures + 1))
        return
    fi

    local denied_session_payload="${tmpdir}/denied-session.json"
    printf '{"firebaseIdToken":"%s"}' "${id_token}" > "${denied_session_payload}"
    chmod 600 "${denied_session_payload}"

    # A verified-but-not-allowlisted UID is rejected with 403 auth_not_allowed
    # (see internal/handlers/auth.go), NOT 401.
    assert_error_response "allowlist-denied-session" "${HTTP_STATUS_FORBIDDEN}" "${ERR_AUTH_NOT_ALLOWED}" \
        -X POST \
        -H 'Content-Type: application/json' \
        --data-binary "@${denied_session_payload}" \
        "${base_url}${AUTH_SESSION_PATH}"
}

run_email_conflict_check() {
    local uid_conflict="${TEST_UID_CONFLICT:-}"
    if [[ -z "${uid_conflict}" ]]; then
        record_result "SKIP" "email-conflict-session" "TEST_UID_CONFLICT not set in shell"
        return
    fi

    if [[ ! -x "${STAGING_ONLY_SEED_SCRIPT}" ]]; then
        record_result "SKIP" "email-conflict-session" "${STAGING_ONLY_SEED_SCRIPT} not found or not executable — seed the fixture manually before running"
        return
    fi

    "${STAGING_ONLY_SEED_SCRIPT}"
    fixture_seeded=true

    local id_token
    if ! id_token="$(firebase_id_token_for_uid "${uid_conflict}")"; then
        record_result "FAIL" "email-conflict-session" "firebase-token failed for TEST_UID_CONFLICT"
        failures=$((failures + 1))
        return
    fi

    local conflict_session_payload="${tmpdir}/conflict-session.json"
    printf '{"firebaseIdToken":"%s"}' "${id_token}" > "${conflict_session_payload}"
    chmod 600 "${conflict_session_payload}"

    assert_error_response "email-conflict-session" "${HTTP_STATUS_CONFLICT}" "${ERR_IDENTITY_CONFLICT}" \
        -X POST \
        -H 'Content-Type: application/json' \
        --data-binary "@${conflict_session_payload}" \
        "${base_url}${AUTH_SESSION_PATH}"
}

run_rate_limit_burst_check() {
    local uid_a="${TEST_UID_A:-}"
    if [[ -z "${uid_a}" ]]; then
        record_result "SKIP" "rate-limit-burst" "TEST_UID_A not set in shell"
        return
    fi

    local id_token
    if ! id_token="$(firebase_id_token_for_uid "${uid_a}")"; then
        record_result "FAIL" "rate-limit-burst" "firebase-token failed for TEST_UID_A"
        failures=$((failures + 1))
        return
    fi

    local burst_payload="${tmpdir}/burst-session.json"
    printf '{"firebaseIdToken":"%s"}' "${id_token}" > "${burst_payload}"
    chmod 600 "${burst_payload}"

    local got_429=false
    local i
    for ((i = 1; i <= BURST_REQUESTS; i++)); do
        local code
        code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 \
            -X POST \
            -H 'Content-Type: application/json' \
            --data-binary "@${burst_payload}" \
            "${base_url}${AUTH_SESSION_PATH}" || true)"
        if [[ "${code}" == "${HTTP_STATUS_TOO_MANY_REQUESTS}" ]]; then
            got_429=true
            break
        fi
    done

    if [[ "${got_429}" == "true" ]]; then
        record_result "PASS" "rate-limit-burst" "received HTTP 429 after burst on /auth/session"
    else
        record_result "FAIL" "rate-limit-burst" "no HTTP 429 after ${BURST_REQUESTS} rapid /auth/session requests"
    fi
}

# --- Main ---

if [[ "${env_name}" == "${ENV_STAGING}" ]]; then
    require_cmd docker
    # trap cleanup EXIT already registered above — fixture_seeded guards the teardown.

    # Staging path: ordered cross-tenant → allowlist-denied → email-conflict → rate-limit burst LAST.
    run_auth_checks
    run_cross_tenant_check
    run_allowlist_denied_check
    run_email_conflict_check
    run_rate_limit_burst_check
    run_log_redaction_check
else
    # Production path: read-only — no /auth/* calls, no mutation, no staging-only paths.
    run_auth_checks
    run_pr1_closed_interval_check
    run_unauthenticated_tenant_check
    run_log_redaction_check
fi

run_finished_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
{
    printf "\nFinished: \`%s\`\n" "${run_finished_at}"
    printf "\nResult: \`%s failure(s)\`\n" "${failures}"
} >> "${report_tmp}"

mv "${report_tmp}" "${report_path}"
printf 'Report written to %s\n' "${report_path}"

if [[ "${failures}" -ne 0 ]]; then
    exit "${EXIT_FAILURE}"
fi
