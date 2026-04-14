#!/usr/bin/env bash
#
# setup-replication-slot.sh
# Creates the logical replication slot on the local PostgreSQL database.
# Run this once before starting the State Sync Engine for the first time.
#
# Usage:
#   ./scripts/setup-replication-slot.sh            # uses defaults
#   DB_HOST=10.0.0.2 DB_NAME=myapp ./scripts/setup-replication-slot.sh
#
set -euo pipefail

DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-demo_app}"
DB_USER="${DB_USER:-postgres}"
SLOT_NAME="${SLOT_NAME:-demo_slot}"

echo "==> Creating logical replication slot '${SLOT_NAME}' on ${DB_HOST}:${DB_PORT}/${DB_NAME}"

psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c \
  "SELECT pg_create_logical_replication_slot('${SLOT_NAME}', 'pgoutput');" 2>/dev/null \
  && echo "    Slot created." \
  || echo "    Slot already exists (or error — check above)."

echo "==> Verifying slot..."
psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c \
  "SELECT slot_name, plugin, slot_type, active FROM pg_replication_slots WHERE slot_name = '${SLOT_NAME}';"

echo "==> Done."
