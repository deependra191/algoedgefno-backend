#!/bin/bash
# Daily NSE EOD sync — runs after market close (7:00 PM IST, Mon–Fri via cron)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "=== NSE EOD sync started at $(date) ===" >> logs/sync.log
./bin/sync >> logs/sync.log 2>&1
echo "=== NSE EOD sync finished at $(date) ===" >> logs/sync.log
