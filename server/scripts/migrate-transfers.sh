#!/usr/bin/env bash
#
# Migrate homelabs-upload (nodevault) history into the merged ServerHub
# database as nv_* tables. Files on disk are NOT copied: point
# FILE_STORAGE_ROOT at the existing data directory instead (trash metadata
# lives inside it and moves along).
#
# Usage:
#   SRC_DSN="postgres://user:pass@host:5432/nodevault?sslmode=disable" \
#   DST_DSN="postgres://user:pass@host:5432/serverhub?sslmode=disable" \
#   ./migrate-transfers.sh
#
# Requirements: psql on PATH; the merged ServerHub binary must have booted
# at least once so GORM created the empty nv_* tables. Sessions are
# intentionally NOT migrated (30-day ephemeral tokens).
#
# Idempotent: re-runs skip already-migrated rows (INSERT ... ON CONFLICT
# DO NOTHING on primary keys). The nodevault database is only ever read.
#
# Rollback: DROP TABLE nv_users, nv_sessions, nv_activity, nv_app_logs;
#
# UNTESTED live — review output counts carefully on first run.
set -Eeuo pipefail

: "${SRC_DSN:?set SRC_DSN to the nodevault DSN}"
: "${DST_DSN:?set DST_DSN to the ServerHub DSN}"

for tbl in users activity app_logs; do
  if ! psql "$SRC_DSN" -tAc "SELECT to_regclass('public.$tbl')" | grep -q "$tbl"; then
    echo "ERROR: source table $tbl missing in nodevault database."
    exit 1
  fi
done

for tbl in nv_users nv_activity nv_app_logs; do
  if ! psql "$DST_DSN" -tAc "SELECT to_regclass('public.$tbl')" | grep -q "$tbl"; then
    echo "ERROR: target table $tbl missing. Boot the merged binary once first (it creates nv_* tables)."
    exit 1
  fi
done

STAGE_CSV="$(mktemp)"
trap 'rm -f "$STAGE_CSV"' EXIT

copy_table() { # copy_table <src> <dst> <columns...>
  local src="$1" dst="$2"
  shift 2
  local cols="$*"
  local before after
  before=$(psql "$DST_DSN" -tAc "SELECT COUNT(*) FROM $dst")
  # Dump to CSV (client side), then load in ONE session: temp staging
  # table (COPY has no upsert) + insert-select with ON CONFLICT DO
  # NOTHING for idempotent re-runs.
  # shellcheck disable=SC2086
  psql "$SRC_DSN" -c "COPY (SELECT $cols FROM $src) TO STDOUT WITH CSV" > "$STAGE_CSV"
  psql "$DST_DSN" <<SQL
CREATE TEMP TABLE _mig_stage (LIKE $dst);
\\copy _mig_stage ($cols) FROM '$STAGE_CSV' CSV
INSERT INTO $dst ($cols) SELECT $cols FROM _mig_stage ON CONFLICT DO NOTHING;
DROP TABLE _mig_stage;
SQL
  after=$(psql "$DST_DSN" -tAc "SELECT COUNT(*) FROM $dst")
  echo "$src -> $dst: before=$before after=$after"
}

# All identifiers quoted: time/user are reserved words in Postgres.
copy_table users nv_users '"id", "username", "display_name", "role", "password_hash", "created_at"'
copy_table activity nv_activity '"time", "user", "role", "action", "path", "detail", "ip", "status"'
copy_table app_logs nv_app_logs '"time", "level", "service", "message", "request_id", "method", "path", "status", "duration_ms", "username", "ip", "fields"'

echo "Migration complete. Verify counts above, then point FILE_STORAGE_ROOT"
echo "at the existing data directory (trash metadata moves with it)."
