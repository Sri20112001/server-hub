# ServerHub — Backend API (Go + Gin + Postgres)

Backend-only implementation of the ServerHub control plane (see repo spec).
Frontend is intentionally **not** included.

## Quick start

```bash
cd server
cp .env.example .env   # then edit secrets
go mod tidy
go run ./cmd/server
# API on http://localhost:4000
```

Default admin: `ADMIN_USERNAME` / `ADMIN_PASSWORD` (seeded on first run).

## API

Auth (cookie `serverhub_session`, http-only; `Authorization: Bearer` also accepted):

```text
POST /server-hub/api/auth/login   {username, password}
POST /server-hub/api/auth/logout
GET  /server-hub/api/auth/me
PUT  /server-hub/api/auth/password  {currentPassword, newPassword} (min 8 chars)
```

```text
GET/POST /server-hub/api/projects            GET/PUT/DELETE /server-hub/api/projects/:id
GET/POST /server-hub/api/projects/:id/services   PUT/DELETE /server-hub/api/services/:id
GET /server-hub/api/containers  GET /server-hub/api/containers/:id  GET /server-hub/api/containers/:id/logs?tail=200&follow=1  GET /server-hub/api/containers/:id/stats
POST /server-hub/api/containers/:id/start
POST /server-hub/api/containers/:id/stop?confirm=true      (high-risk, confirmation required)
POST /server-hub/api/containers/:id/restart?confirm=true   (high-risk, confirmation required)
POST /server-hub/api/projects/:id/start
POST /server-hub/api/projects/:id/stop?confirm=true       (high-risk, docker compose stop)
POST /server-hub/api/projects/:id/restart?confirm=true    (high-risk, docker compose restart)
GET /server-hub/api/images  GET /server-hub/api/volumes
GET /server-hub/api/discovery (Docker labels + directory signatures under SCAN_ROOTS, default /srv/apps)
POST /server-hub/api/discovery/import  {name} (register ship + stations; compose services parsed on import)
GET /server-hub/api/health  GET /server-hub/api/projects/:id/health
GET /server-hub/api/deployments  GET/POST /server-hub/api/projects/:id/deployments  POST /server-hub/api/projects/:id/deploy
POST /server-hub/api/projects/:id/deployments/:depId/rollback (re-run compose for a past launch)
DELETE /server-hub/api/deployments (disabled: 410 — deployment history is append-only and cannot be wiped; attempts are audit-logged)
GET /server-hub/api/telemetry?range=15m|1h|6h|24h  GET /server-hub/api/telemetry/latest (per-minute host samples, retained forever — append-only)
GET /server-hub/api/events (SSE live bus: deployment.*, container.*, project.*, health.changed, gateway.reloaded, backup.*, telemetry.threshold, discovery.completed)
GET /server-hub/api/operations?limit=50  GET /server-hub/api/operations/:id (unified progress for deploys, restarts, backups…)
POST /server-hub/api/containers/:id/exec?confirm=true {shell} (single-use token) → WS /server-hub/api/exec/:token (docker-exec PTY, audit-logged)
GET/POST /server-hub/api/projects/:id/backups  GET/DELETE /server-hub/api/backups/:id  POST /server-hub/api/backups/:id/restore?confirm=true (tar.gz snapshots + manifest)
GET /server-hub/api/logs (central append-only app_logs store — all activity, read-only)
GET/POST /server-hub/api/projects/:id/secrets  PUT /server-hub/api/secrets/:id (rotate)  DELETE /server-hub/api/secrets/:id  POST /server-hub/api/secrets/:id/reveal
GET/POST /server-hub/api/gateway/routes  PUT/DELETE /server-hub/api/gateway/routes/:id  POST /server-hub/api/gateway/validate  POST /server-hub/api/gateway/reload
GET /server-hub/api/server  GET /server-hub/api/dashboard  GET /server-hub/api/audit
GET /health (public liveness)
POST /server-hub/api/webhooks/github (public, HMAC-signed — see below)
```

Notes:

- All logs live in the DB and are append-only: every audit event is mirrored
  into the central `app_logs` table; `app_logs`/`audit_logs` reject UPDATE and
  DELETE via Postgres triggers, and `deployments`/`backups`/
  `operations`/`server_snapshots` reject DELETE. There is no prune/wipe:
  `applog.Prune` is a no-op, `DELETE /deployments` returns 410, backup delete
  reclaims only the archive file (the DB row + logs are retained as DELETED),
  and telemetry snapshots use insert-only writes. `GET /logs` and `GET /audit`
  are read-only.
- Secrets API returns metadata only; values are AES-256-GCM encrypted at rest in Postgres and
  revealed only via `POST /server-hub/api/secrets/:id/reveal` (audit-logged, value never in logs).
  Rotate with `PUT /server-hub/api/secrets/:id {value}`.
- Docker is read-only except lifecycle/deploy endpoints. Stop/restart (container and
  project level) are high-risk and require `?confirm=true` or body `{"confirm": true}`;
  without it the API returns 400 and runs nothing. Every lifecycle action is audit-logged.
  Without `/var/run/docker.sock`, container endpoints return `503` with a clear message.
- `POST /server-hub/api/projects/:id/deploy` runs `docker compose pull && up -d` inside the
  project's `deployment_path` and records the deployment.
- GitHub webhooks: set `GITHUB_WEBHOOK_SECRET` and point a repo webhook at
  `POST /server-hub/api/webhooks/github` (push events). The signature (`X-Hub-Signature-256`)
  is verified; pushes are matched to projects by repository + branch. Projects with
  `autoDeploy: true` deploy immediately; others get a `PENDING` deployment row for
  visibility without running anything.
- Gateway `validate`/`reload` shell out to `caddy` if installed; otherwise basic checks.
- Health checker polls `health_url` every `HEALTH_CHECK_INTERVAL_SEC` (default 60s).

## Docker

```bash
cd server
docker compose up --build -d
```

Mounts: `./data` (backups), `/var/run/docker.sock` (ro), Caddyfile (ro).

## Env

See `.env.example`. `DATABASE_URL` is required (Postgres is the only
supported database). `SERVERHUB_ENCRYPTION_KEY` must be stable 64-char hex
in production.

## Tests

DB-backed tests need a live Postgres; each test gets an isolated
`serverhub_test_*` database that is dropped afterwards. Without a reachable
Postgres the DB tests skip (set `TEST_DATABASE_URL` to run them):

```bash
cd server
TEST_DATABASE_URL=postgres://serverhub:changeme@localhost:5432/postgres?sslmode=disable go test ./...
```
