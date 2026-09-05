#!/bin/sh
# Restore a database from a dump file.
# Usage: docker compose --profile tools run --rm \
#   -v /path/to/dumps:/dumps backup \
#   sh /restore.sh <database_name> /dumps/<file>.dump
set -e

DB="$1"
DUMP="$2"

if [ -z "$DB" ] || [ -z "$DUMP" ]; then
  echo "Usage: restore.sh <database_name> <dump_file>"
  echo "  e.g.: restore.sh expenselog /backups/expenselog-20260904-120000.dump"
  exit 1
fi

if [ ! -f "$DUMP" ]; then
  echo "[restore] ERROR: file not found: $DUMP"
  exit 1
fi

echo "[restore] restoring $DB from $DUMP ..."
pg_restore --clean --if-exists -h "$PGHOST" -U "$PGUSER" -d "$DB" "$DUMP"
echo "[restore] $DB restored successfully"
