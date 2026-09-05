#!/bin/sh
# On-demand backup of both ExpenseLog databases.
# Run with: docker compose --profile tools run --rm backup
set -e

STAMP=$(date +%Y%m%d-%H%M%S)

echo "[backup] starting dump at ${STAMP}"

pg_dump -Fc -d expenselog      > "/backups/expenselog-${STAMP}.dump"
echo "[backup] expenselog dumped"

pg_dump -Fc -d expenselog_bot  > "/backups/expenselog_bot-${STAMP}.dump"
echo "[backup] expenselog_bot dumped"

# Retain only the last 7 days of backups
find /backups -name '*.dump' -mtime +7 -delete 2>/dev/null || true

echo "[backup] done: ${STAMP}"
