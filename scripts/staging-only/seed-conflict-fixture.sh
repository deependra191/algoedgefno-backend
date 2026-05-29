#!/bin/bash
# seed-conflict-fixture.sh — staging-only SQL fixture for the identity_conflict
# abuse test. Inserts a users row with a pre-existing firebase_uid that is NOT
# in ALLOWED_FIREBASE_UIDS so that a sign-in attempt by TEST_UID_CONFLICT
# (whose Firebase email is conflict@test.algoedge.local) returns 409.
#
# This script is staging-only. It must never run against the production database.
# It is idempotent: ON CONFLICT (email) DO NOTHING means repeated runs are safe.
#
# Usage:
#   seed-conflict-fixture.sh [--teardown]
#
#   --teardown   Remove the fixture row. Safe to call multiple times.
#
# Required environment:
#   COMPOSE_DIR       Path to the compose directory (default: /opt/algoedgefno/compose)
#   STAGING_DB_NAME   Staging database name (default: algoedgefno_staging)
set -euo pipefail

readonly STAGING_FIXTURE_EMAIL="conflict@test.algoedge.local"
readonly STAGING_FIXTURE_PRE_UID="STAGING_FIXTURE_PRE_UID"

COMPOSE_DIR="${COMPOSE_DIR:-/opt/algoedgefno/compose}"
STAGING_DB_NAME="${STAGING_DB_NAME:-algoedgefno_staging}"
teardown=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --teardown)
            teardown=true
            shift
            ;;
        -h|--help)
            printf 'usage: %s [--teardown]\n' "$0" >&2
            exit 0
            ;;
        *)
            printf 'unknown argument: %s\n' "$1" >&2
            exit 2
            ;;
    esac
done

fail() {
    printf 'FAIL %s\n' "$1" >&2
    exit 1
}

pass() {
    printf 'PASS %s\n' "$1"
}

run_sql() {
    local sql="$1"
    (
        cd "${COMPOSE_DIR}"
        docker compose exec -T postgres sh -c \
            'psql -U "$POSTGRES_USER" -d "$1" -tA -c "$2"' \
            sh "${STAGING_DB_NAME}" "${sql}"
    )
}

if [[ "${teardown}" == "true" ]]; then
    run_sql "DELETE FROM users WHERE email = '${STAGING_FIXTURE_EMAIL}' AND firebase_uid = '${STAGING_FIXTURE_PRE_UID}';" > /dev/null
    pass "seed-conflict-fixture: teardown — removed fixture row for ${STAGING_FIXTURE_EMAIL}"
    exit 0
fi

# Seed the fixture row idempotently. STAGING_FIXTURE_PRE_UID is not in
# ALLOWED_FIREBASE_UIDS, so an attempt by TEST_UID_CONFLICT (same email)
# will hit the identity_conflict path in the user store.
run_sql "INSERT INTO users (id, firebase_uid, email, last_login_at, created_at, updated_at)
VALUES (gen_random_uuid(), '${STAGING_FIXTURE_PRE_UID}',
        '${STAGING_FIXTURE_EMAIL}', NOW(), NOW(), NOW())
ON CONFLICT (email) DO NOTHING;" > /dev/null

pass "seed-conflict-fixture: seeded fixture row for ${STAGING_FIXTURE_EMAIL}"
