#!/bin/bash
set -euo pipefail

readonly ENV_STAGING="staging"
readonly ENV_PROD="prod"
readonly DEFAULT_STAGING_BASE_URL="https://staging-api.algoedgefno.com"
readonly DEFAULT_PROD_BASE_URL="https://api.algoedgefno.com"
readonly DEFAULT_STAGING_CONTAINER="algoedgefno-backend-staging"
readonly DEFAULT_PROD_CONTAINER="algoedgefno-backend-prod"
readonly REPORT_DIR="scratch/security-runs"

readonly PROTECTED_CONFIG_PATH="/api/v1/config/app"
readonly BACKTESTS_PATH="/api/v1/backtests"
readonly POLL_BACKTEST_ID="11111111-1111-4111-8111-111111111111"

readonly DEFAULT_TEST_STRATEGY="ma_crossover"
readonly DEFAULT_TEST_UNDERLYING="NIFTY"
readonly DEFAULT_TEST_LOTS=1
readonly DEFAULT_TEST_CAPITAL=200000
readonly DEFAULT_TEST_FROM_OFFSET_DAYS=37
readonly DEFAULT_TEST_TO_OFFSET_DAYS=7
readonly LARGE_RANGE_FROM="2015-01-01"
readonly LARGE_RANGE_TO="2026-01-01"

readonly BURST_REQUESTS=50
readonly POLL_INTERVAL_SECONDS="0.1"
readonly POLL_DURATION_SECONDS=30
readonly POLL_REQUESTS=300

readonly EXIT_USAGE=2
readonly EXIT_FAILURE=1

readonly HTTP_STATUS_OK=200
readonly HTTP_STATUS_ACCEPTED=202
readonly HTTP_STATUS_BAD_REQUEST=400
readonly HTTP_STATUS_UNAUTHORIZED=401
readonly HTTP_STATUS_NOT_FOUND=404
readonly HTTP_STATUS_UNPROCESSABLE_ENTITY=422
readonly HTTP_STATUS_TOO_MANY_REQUESTS=429
readonly HTTP_STATUS_SERVICE_UNAVAILABLE=503
readonly HTTP_STATUS_SERVER_ERROR_PREFIX_REGEX='^5'

readonly ERR_MISSING_AUTH="missing or invalid authorization header"
readonly ERR_DATE_RANGE_EXCEEDED="date range exceeds maximum allowed"
readonly ERR_BACKTEST_DISABLED="backtests are disabled"
readonly ERR_NO_CANDLE_DATA="no candle data available"
readonly ERR_NO_INSTRUMENT="no instrument found for underlying"
readonly ERR_CANDLE_COUNT_EXCEEDED="candle count exceeds maximum allowed"

usage() {
    cat >&2 <<'EOF'
usage: scripts/security/abuse-suite.sh --env staging|prod [--expect-backtests-disabled]

Runs deterministic HTTP abuse checks and writes a sanitized markdown report under
scratch/security-runs/YYYY-MM-DD-{env}.md. The suite never reads server env
files and never prints bearer token values.
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
expect_backtests_disabled=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --env)
            [[ $# -ge 2 ]] || fail_usage "--env requires a value"
            env_name="$2"
            shift 2
            ;;
        --expect-backtests-disabled)
            expect_backtests_disabled=true
            shift
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
        token_var_name="STAGING_APP_TOKEN"
        app_token="${STAGING_APP_TOKEN:-}"
        ;;
    "${ENV_PROD}")
        base_url="${ABUSE_PROD_BASE_URL:-${DEFAULT_PROD_BASE_URL}}"
        container_name="${ABUSE_PROD_CONTAINER:-${DEFAULT_PROD_CONTAINER}}"
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

if [[ "${env_name}" == "${ENV_PROD}" && "${expect_backtests_disabled}" == "true" ]]; then
    fail_usage "--expect-backtests-disabled is staging-only"
fi

if [[ -z "${app_token}" ]]; then
    fail_fast "${token_var_name} must be set in the shell"
fi

require_cmd curl
require_cmd python3

tmpdir="$(mktemp -d)"
auth_configs=()
secret_files=()
report_lock=""

cleanup() {
    local cfg
    for cfg in "${auth_configs[@]}"; do
        [[ -n "${cfg}" ]] && rm -f "${cfg}"
    done
    for cfg in "${secret_files[@]}"; do
        [[ -n "${cfg}" ]] && rm -f "${cfg}"
    done
    [[ -n "${report_lock}" && -d "${report_lock}" ]] && rmdir "${report_lock}" 2>/dev/null || true
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

test_strategy="${ABUSE_TEST_STRATEGY:-${DEFAULT_TEST_STRATEGY}}"
test_underlying="${ABUSE_TEST_UNDERLYING:-${DEFAULT_TEST_UNDERLYING}}"
test_from="${ABUSE_TEST_FROM:-}"
test_to="${ABUSE_TEST_TO:-}"

default_date() {
    local offset_days="$1"
    python3 - "$offset_days" <<'PY'
from datetime import datetime, timedelta, timezone
import sys

offset = int(sys.argv[1])
print((datetime.now(timezone.utc) - timedelta(days=offset)).strftime("%Y-%m-%d"))
PY
}

if [[ -z "${test_from}" ]]; then
    test_from="$(default_date "${DEFAULT_TEST_FROM_OFFSET_DAYS}")"
fi
if [[ -z "${test_to}" ]]; then
    test_to="$(default_date "${DEFAULT_TEST_TO_OFFSET_DAYS}")"
fi

markdown_cell() {
    local value="$1"
    value="${value//$'\n'/ }"
    value="${value//|/\\/}"
    printf '%s' "${value}"
}

{
    printf '# Security Abuse Suite Report\n\n'
    printf -- "- Started: \`%s\`\n" "${run_started_at}"
    printf -- "- Environment: \`%s\`\n" "${env_name}"
    printf -- "- Base URL: \`%s\`\n" "${base_url}"
    printf -- "- Container: \`%s\`\n" "${container_name}"
    printf -- "- Backtests disabled mode: \`%s\`\n" "${expect_backtests_disabled}"
    printf -- "- Test payload: strategy \`%s\`, underlying \`%s\`, from \`%s\`, to \`%s\`\n\n" \
        "${test_strategy}" "${test_underlying}" "${test_from}" "${test_to}"
    printf '| Status | Test | Detail |\n'
    printf '|---|---|---|\n'
} > "${report_tmp}"

failures=0

record_result() {
    local status="$1"
    local name="$2"
    local detail="$3"
    printf '| %s | %s | %s |\n' \
        "$(markdown_cell "${status}")" \
        "$(markdown_cell "${name}")" \
        "$(markdown_cell "${detail}")" >> "${report_tmp}"
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

selected_auth_cfg="$(auth_config "${app_token}" "selected")"
invalid_auth_cfg="$(auth_config "invalid-abuse-suite-token" "invalid")"

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
        record_result "FAIL" "${test_name}" "got HTTP ${code}, want ${expected_status}; body saved only in temp workspace"
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
        record_result "FAIL" "${test_name}" "got HTTP ${code} error '${actual_error}', want HTTP ${expected_status} expected error JSON"
    fi
}

write_backtest_payload() {
    local output="$1"
    local from="$2"
    local to="$3"
    python3 - "$output" "$test_strategy" "$test_underlying" "$from" "$to" "${DEFAULT_TEST_LOTS}" "${DEFAULT_TEST_CAPITAL}" <<'PY'
import json
import sys

path, strategy, underlying, from_date, to_date, lots, capital = sys.argv[1:8]
payload = {
    "strategyId": strategy,
    "inputs": {
        "underlying": underlying,
        "dateRange": {"from": from_date, "to": to_date},
        "lots": int(lots),
        "capital": float(capital),
    },
}
with open(path, "w", encoding="utf-8") as f:
    json.dump(payload, f, separators=(",", ":"))
PY
}

submit_backtest() {
    local name="$1"
    local from="$2"
    local to="$3"
    local payload="${tmpdir}/${name}.json"
    write_backtest_payload "${payload}" "${from}" "${to}"
    http_request "${name}" \
        -X POST \
        -H 'Content-Type: application/json' \
        --data-binary "@${payload}" \
        --config "${selected_auth_cfg}" \
        "${base_url}${BACKTESTS_PATH}"
}

run_auth_checks() {
    assert_error_response "protected-no-auth" "${HTTP_STATUS_UNAUTHORIZED}" "${ERR_MISSING_AUTH}" \
        "${base_url}${PROTECTED_CONFIG_PATH}"

    assert_error_response "protected-invalid-token" "${HTTP_STATUS_UNAUTHORIZED}" "${ERR_MISSING_AUTH}" \
        --config "${invalid_auth_cfg}" \
        "${base_url}${PROTECTED_CONFIG_PATH}"

    assert_status "protected-valid-token" "${HTTP_STATUS_OK}" \
        --config "${selected_auth_cfg}" \
        "${base_url}${PROTECTED_CONFIG_PATH}"

    if [[ "${env_name}" == "${ENV_PROD}" ]]; then
        if [[ -n "${STAGING_APP_TOKEN:-}" ]]; then
            local staging_auth_cfg
            staging_auth_cfg="$(auth_config "${STAGING_APP_TOKEN}" "staging-cross-env")"
            assert_error_response "prod-url-staging-token" "${HTTP_STATUS_UNAUTHORIZED}" "${ERR_MISSING_AUTH}" \
                --config "${staging_auth_cfg}" \
                "${base_url}${PROTECTED_CONFIG_PATH}"
        else
            record_result "SKIP" "prod-url-staging-token" "STAGING_APP_TOKEN not set in shell"
        fi
    fi
}

# run_large_range_or_disabled_check: SKIP in PR 1 closed-interval — tenant endpoints return
# 401 to the static token; PR 2 reintroduces this test via Firebase JWT.
run_large_range_or_disabled_check() {
    record_result "SKIP" "backtest-large-date-range" "PR 1 closed-interval — tenant endpoints return 401 to static token; PR 2 reintroduces this test via Firebase JWT"
}

is_clean_burst_response() {
    local code="$1"
    local body="$2"
    local actual_error
    case "${code}" in
        "${HTTP_STATUS_ACCEPTED}"|"${HTTP_STATUS_TOO_MANY_REQUESTS}")
            return 0
            ;;
        "${HTTP_STATUS_BAD_REQUEST}"|"${HTTP_STATUS_NOT_FOUND}"|"${HTTP_STATUS_UNPROCESSABLE_ENTITY}")
            actual_error="$(json_error_value "${body}")"
            case "${actual_error}" in
                "${ERR_NO_CANDLE_DATA}"|"${ERR_NO_INSTRUMENT}"|"${ERR_CANDLE_COUNT_EXCEEDED}")
                    return 0
                    ;;
            esac
            ;;
    esac
    return 1
}

# run_burst_check: SKIP in PR 1 closed-interval — tenant endpoints return 401 to the static
# token; PR 2 reintroduces this test via Firebase JWT.
run_burst_check() {
    record_result "SKIP" "burst-backtest-submit" "PR 1 closed-interval — tenant endpoints return 401 to static token; PR 2 reintroduces this test via Firebase JWT"
}

is_clean_poll_response() {
    local code="$1"
    case "${code}" in
        "${HTTP_STATUS_OK}"|"${HTTP_STATUS_BAD_REQUEST}"|"${HTTP_STATUS_NOT_FOUND}"|"${HTTP_STATUS_TOO_MANY_REQUESTS}")
            return 0
            ;;
    esac
    return 1
}

# run_poll_check: SKIP in PR 1 closed-interval — tenant endpoints return 401 to the static
# token; PR 2 reintroduces this test via Firebase JWT.
run_poll_check() {
    record_result "SKIP" "aggressive-result-poll" "PR 1 closed-interval — tenant endpoints return 401 to static token; PR 2 reintroduces this test via Firebase JWT"
}

# run_pr1_closed_interval_check: asserts that the static APP_SECRET_TOKEN is rejected (401)
# on all tenant endpoints after PR 1 deploys. /api/v1/config/app 200 is already covered by
# run_auth_checks. Uses BACKTESTS_PATH and HTTP_STATUS_UNAUTHORIZED constants (rule 17);
# uses auth_config curl helper to avoid token-in-URL leakage (rule 4).
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

run_auth_checks
run_pr1_closed_interval_check
run_large_range_or_disabled_check
record_result "SKIP" "cross-tenant-strategy-backtest-id-lookup" "PR 1 closed-interval — no JWT path exists; PR 2 implements real cross-tenant test via Firebase JWT"
run_burst_check
run_poll_check
run_log_redaction_check

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
