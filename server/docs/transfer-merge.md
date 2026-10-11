# File-transfer merge (homelabs-upload → ServerHub)

Single ServerHub binary + single Postgres + single UI. The homelabs-upload
repo is untouched and still builds/runs standalone.

## What lives where

- `server/internal/transfer/` — ported file service (path-safe filesystem
  ops, trash, zip, chunked uploads, search, storage stats), HTTP handlers,
  `nv_*` history helpers. No homelabs imports; no dependency conflicts.
- `nv_users`, `nv_sessions`, `nv_activity`, `nv_app_logs` — namespaced
  legacy tables, auto-created on boot like every other model.
- API: `/server-hub/api/transfers/*` (reads → viewer, mutations →
  operator). Identity is ServerHub JWT only; there is no file-service login.
- UI: `/transfers` route + Dock item (full file browser, ported).
- Config: `FILE_STORAGE_ROOT` (empty = endpoints off), `FILE_MAX_UPLOAD_SIZE`.

## Migration (one time, operator-run)

Preconditions: merged binary booted once (empty `nv_*` tables exist);
`psql` on PATH; DSNs in environment (never in the script).

```sh
SRC_DSN="postgres://user:pass@host:5432/nodevault?sslmode=disable" \
DST_DSN="postgres://user:pass@host:5432/serverhub?sslmode=disable" \
./scripts/migrate-transfers.sh
```

What it does: copies `users`/`activity`/`app_logs` → `nv_*` via CSV
staging + `INSERT ... ON CONFLICT DO NOTHING` (idempotent re-runs;
nodevault only ever read). Sessions are skipped (ephemeral). Files on
disk are NOT copied — set `FILE_STORAGE_ROOT` to the existing data
directory (trash `.meta` sidecars move with it).

Verify: compare the before/after counts the script prints; spot-check
`/transfers/activity` in the UI; confirm a legacy username appears on its
old actions.

Rollback: `DROP TABLE nv_users, nv_sessions, nv_activity, nv_app_logs;`
and point `FILE_STORAGE_ROOT` back (or unset it). The nodevault database
and homelabs deployment are never modified by migration or rollback.

## What was deliberately NOT merged

- homelabs `users`/`sessions` login flow (ServerHub owns identity now).
- homelabs config/users/activity-admin endpoints (ServerHub equivalents
  exist); SSE directory watch (UI polls instead).
- Databases stay separate products: the script copies history rows only.
