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

Auth (cookie `serverhub_session`, http-only + `SameSite=Lax`; `Authorization: Bearer` also accepted):

```text
POST /server-hub/api/auth/login   {username, password}  (rate-limited; sets session + refresh cookies)
POST /server-hub/api/auth/token   {username, password}  (mobile/API: returns token + refreshToken)
POST /server-hub/api/auth/refresh (cookie serverhub_refresh or {refreshToken}; rotates)
POST /server-hub/api/auth/logout  (revokes this refresh token)
POST /server-hub/api/auth/logout-all (revokes every session for the caller)
GET  /server-hub/api/auth/me
PUT  /server-hub/api/auth/password  {currentPassword, newPassword} (min 8 chars; kills all sessions)
```

Sessions: access tokens live 30 minutes; refresh tokens live 7 days and
rotate on every use. Reusing a revoked refresh token revokes ALL sessions
for that user (theft response). Logins are throttled: 5 attempts/min/IP plus
per-account lockout with doubling backoff; 429 responses carry Retry-After.

Users & roles (admin only; last-admin demote/delete is refused):

```text
GET/POST /server-hub/api/users   {username, password (min 8), role}
PUT /server-hub/api/users/:username/role  {role}
DELETE /server-hub/api/users/:username
```

Roles (viewer < operator < admin):

```text
viewer   read-only: dashboards, telemetry, logs, metadata
operator viewer + projects/services, deploys, rollbacks, backups, secret upsert, discovery imports
admin    operator + container/project stop/restart, exec shells, secret rotate/delete/reveal,
         notification settings, user management
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
GET /server-hub/api/events (SSE live bus: deployment.*, container.*, project.*, health.changed, backup.*, telemetry.threshold, discovery.completed)
GET /server-hub/api/operations?limit=50  GET /server-hub/api/operations/:id (unified progress for deploys, restarts, backups…)
POST /server-hub/api/containers/:id/exec?confirm=true {shell} (single-use token) → WS /server-hub/api/exec/:token (docker-exec PTY, audit-logged)
GET/POST /server-hub/api/projects/:id/backups  GET/DELETE /server-hub/api/backups/:id  POST /server-hub/api/backups/:id/restore?confirm=true (tar.gz snapshots + manifest)
GET /server-hub/api/logs (central append-only app_logs store — all activity, read-only)
GET/POST /server-hub/api/projects/:id/secrets  PUT /server-hub/api/secrets/:id (rotate, admin)  DELETE /server-hub/api/secrets/:id (admin)  POST /server-hub/api/secrets/:id/reveal (admin)
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
- Health checker polls `health_url` every `HEALTH_CHECK_INTERVAL_SEC` (default 60s).
- Discovery scans are shallow (one level below each `SCAN_ROOTS` entry) and
  bounded: max 32 roots, 1000 entries per directory, 200 projects, 20s total.

## Docker

```bash
cd server
docker compose up --build -d
```

Mounts: `./data` (backups), `/var/run/docker.sock` (ro), Caddyfile (ro).

Postgres is published on **127.0.0.1 only** (see `docker-compose.yml`) and
`POSTGRES_PASSWORD` has **no default** — Compose fails closed without it.
Copy `server/.env.example` to `server/.env` and set a strong password.

## Security model

Read this before exposing ServerHub beyond localhost.

```text
Internet
  ↓ HTTPS (port 443)
TLS reverse proxy (Caddy / Apache / Nginx)
  ↓ plain HTTP over localhost or the private Compose network
ServerHub :4000 ──/var/run/docker.sock──▶ Docker daemon (HOST ROOT)
  ↓
Postgres (127.0.0.1:5432, Compose network)
```

Rules:

1. **Never expose `:4000` or Postgres to the Internet.** Terminate TLS at
   the reverse proxy and set `COOKIE_SECURE=true` (cookies are HttpOnly +
   `SameSite=Lax`).
2. **The Docker socket is host-root-equivalent.** The `:ro` mount flag does
   NOT make the Docker API read-only: anyone who can call container
   start/stop/exec/deploy through the API effectively controls the host.
   Treat compromise of ServerHub (or of any `admin` account) as compromise
   of every Docker workload on the host. There is intentionally no
   privilege separation between the API and the socket — this product is a
   homelab control plane, not a multi-tenant platform.
3. **Roles are enforced at the router** (`viewer` < `operator` < `admin`,
   unknown roles denied). High-risk endpoints additionally require
   `?confirm=true`: container/project stop/restart, exec shell minting,
   snapshot restore. Every lifecycle action is audit-logged.
4. **Logins are throttled** (5/min/IP + per-account doubling lockout) and
   lockouts are audit-logged. Access tokens live 30 minutes; 7-day refresh
   tokens rotate on each use and reuse revokes all sessions for the user.
5. **Secrets are AES-256-GCM** at rest; the 64-char hex
   `SERVERHUB_ENCRYPTION_KEY` must be stable — losing it makes stored
   secrets undecryptable. Values are only ever returned by the admin-only
   `reveal` endpoint and never appear in logs (request logging stores
   method/path/status only, never bodies).
6. **Logs are append-only by design** (`app_logs`/`audit_logs` reject
   UPDATE/DELETE via triggers; history tables reject DELETE). There is no
   prune/wipe API. Plan disk capacity accordingly: per-minute telemetry +
   per-request rows grow monotonically. List endpoints are paginated
   (`limit` clamped server-side); time-based partitioning is future work.

## Env

See `.env.example` (`cp .env.example .env`, then fill in real secrets — never
commit `.env`). `DATABASE_URL` is required (Postgres is the only
supported database). `SERVERHUB_ENCRYPTION_KEY` must be stable 64-char hex
in production. `JWT_SECRET` must be at least 32 characters.

## Tests

DB-backed tests need a live Postgres; each test gets an isolated
`serverhub_test_*` database that is dropped afterwards. Without a reachable
Postgres the DB tests skip (set `TEST_DATABASE_URL` to run them):

```bash
cd server
TEST_DATABASE_URL=postgres://serverhub:changeme@localhost:5432/postgres?sslmode=disable go test ./...
```
