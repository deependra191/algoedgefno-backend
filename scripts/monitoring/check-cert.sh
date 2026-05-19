#!/bin/bash
# Checks the TLS certificate expiry for a given hostname via live TLS handshake.
# Usage: check-cert.sh <hostname>
# Exits 0 if the cert has >= 14 days remaining.
# Exits 1 with a one-line stderr message if expiry is imminent or the check fails.
set -euo pipefail

readonly WARN_DAYS=14

host="${1:?usage: check-cert.sh <hostname>}"

end_date=$(timeout 8 bash -c "echo | openssl s_client -connect \"${host}:443\" -servername \"${host}\" 2>/dev/null \
    | openssl x509 -noout -enddate" | cut -d= -f2) || {
    printf 'cert_%s: TLS handshake failed for %s\n' "${host}" "${host}" >&2
    exit 1
}

expiry_epoch=$(date -d "${end_date}" +%s 2>/dev/null) || {
    printf 'cert_%s: could not parse cert date for %s\n' "${host}" "${host}" >&2
    exit 1
}

now_epoch=$(date +%s)
days_remaining=$(( (expiry_epoch - now_epoch) / 86400 ))

if [[ "${days_remaining}" -lt "${WARN_DAYS}" ]]; then
    printf '%sd until expiry for %s\n' "${days_remaining}" "${host}" >&2
    exit 1
fi

exit 0
