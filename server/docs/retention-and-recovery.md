# Retention policy and disaster recovery

Single-operator assumption (see README): one trusted operator manages their
own servers. The policies below match that model; multi-team use needs
per-resource authorization first.

## 1. Retention policy by log category

| Category | Store | Policy | Rationale |
|---|---|---|---|
| Security audit events (`audit_logs`) | Postgres, append-only (triggers reject UPDATE/DELETE) | **Retain indefinitely** | Tamper-evident record; volume is low (one row per action) |
| Operational app logs (`app_logs`, source=api/system) | Postgres, guarded (no UPDATE; DELETE only past 30d floor) | `APPLOG_RETENTION_DAYS` (default 90, min 30), pruned hourly in bounded batches | High volume (per-request rows); needed for recent debugging only |
| Deployment output (`deployments.logs`, op logs) | Postgres, delete-protected | **Retain with the deployment row** (append-only) | Release history must stay queryable |
| Monitoring history (`server_metrics`) | Postgres, pruned hourly | `SERVER_METRICS_RETENTION_DAYS` (default 30, max 365) | Bounds the largest table; enforced by code today |
| Health-check results (`health_check_results`) | Postgres, pruned hourly | `HEALTHCHECK_RESULTS_RETENTION_DAYS` (default 30, min 1) | Raw probe history ages fast; alerts persist separately |
| Backup archives (files) | `BACKUP_DIR` on disk | Keep last **10 SUCCESS** per project + all `pre-restore` snapshots for 30 days | Disk-bounded, recovery-capable |
| Backup records (rows) | Postgres, delete-protected | **Retain indefinitely** (file reclaimed, row marked DELETED) | Provenance for every restore |

> Design note (reviewed policy change): `audit_logs` remains fully
> immutable with no exception. `app_logs` moved from fully-immutable to an
> age-floor guard — UPDATE always rejected, DELETE allowed only for rows
> older than 30 days (`AppLogsPruneFloorDays`, enforced by the trigger
> itself, so a misconfigured retention fails loudly instead of wiping
> fresh history). Prune jobs delete in bounded ctid batches and log
> counts/durations per run. `deployments`, `backups`, `operations` stay
> delete-protected; nothing prunes them.

## 2. What to back up (all three, together)

A database dump alone is **not** a recovery plan: project secrets and
database credentials are AES-256-GCM encrypted with
`SERVERHUB_ENCRYPTION_KEY`. A restore with a different key yields a
working app with permanently unreadable secrets.

1. **PostgreSQL dump** (nightly minimum):
   `pg_dump -Fc -h 127.0.0.1 -p <port> -U serverhub serverhub > serverhub-$(date +%F).dump`
2. **Encryption key + configuration**: the 64-char hex
   `SERVERHUB_ENCRYPTION_KEY`, `JWT_SECRET`, `POSTGRES_PASSWORD`,
   `ADMIN_PASSWORD`, webhook secrets — sealed (password manager / sealed
   envelope), stored **separately** from the database dump.
3. **Backup archives** (`BACKUP_DIR` contents) if project-file recovery
   outside the database matters to you.

## 3. Recovery rehearsal (run quarterly, in isolation)

1. Provision an isolated host/VM with Docker. No production credentials.
2. `pg_restore` the dump into a fresh Postgres; place the archived
   `BACKUP_DIR` alongside it.
3. Set the **archived** `SERVERHUB_ENCRYPTION_KEY` (not a new one) and the
   remaining archived secrets; boot with `SERVERHUB_ENV=production`.
4. Verify: `/health/ready` 200, admin login works.
5. Verify: reveal one stored project secret and one saved database
   credential — decryption proves key/dump match.
6. Verify: list deployments/backups, download one archive, restore one
   backup into a scratch `DEPLOY_ROOTS` and confirm contents.
7. Record pass/fail with dump date + image digest; fix gaps before the
   next release.

Acceptance: steps 4–6 all pass from archived artifacts alone.

## 5. Rehearsal record (fill in per run)

Declare objectives first, then measure against them:

- **RPO** (max acceptable data loss): __ (e.g. 24h — one nightly dump).
- **RTO** (max acceptable recovery time): __ (e.g. 4h).

| Field | Value |
|---|---|
| Date | |
| Dump date / age (= measured data loss vs RPO) | |
| Image digest restored | |
| Time to ready (dump start → `/health/ready` 200) vs RTO | |
| Admin login | pass / fail |
| Secret reveal (decryption proof) | pass / fail |
| Project/backup list + archive download | pass / fail |
| Scratch restore into isolated `DEPLOY_ROOTS` | pass / fail |
| Gaps found | |
| Next rehearsal due | |

## 4b. Deployment rollback limits

Schema migrations are **additive-only**: tables/columns are created,
never altered destructively, and there are no down-migrations. Rolling
back the application image does **not** roll back schema changes (new
columns/tables simply go unused by the old code, which is safe because
additions are backward compatible). The pre-deploy database dump taken by
CI (`pg-backup-<build>.sql.gz`) is the data rollback path: restore it
with `pg_restore` only if a migration itself caused the failure.

## 4. Emergency recovery

- **Lost encryption key**: secrets are unrecoverable. Rotate the key
  (new stable value), re-enter every project secret and database
  credential, revoke all sessions (`logout-all`), rotate webhook secrets.
- **Lost database, key survives**: restore dump per §3; secrets decrypt
  with the surviving key.
- **Lost database and key**: rebuild from scratch; treat all previously
  stored credentials as compromised and rotate them at their sources.
