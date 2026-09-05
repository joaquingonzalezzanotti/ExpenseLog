#!/bin/bash
# Creates the two logical databases on first PostgreSQL boot.
# Mounted as /docker-entrypoint-initdb.d/init-db.sh (read-only).
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres <<-EOSQL
    SELECT 'CREATE DATABASE expenselog'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'expenselog')\gexec

    SELECT 'CREATE DATABASE expenselog_bot'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'expenselog_bot')\gexec
EOSQL

echo "[init-db] databases ready"
