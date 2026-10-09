# Staging smoke checklist

Run against a staging deployment built from the **same commit and image
digest** intended for release (`image-digest.txt` from the Security stage).
All checks must pass; record the digest with the results.

## Auth & sessions

- [ ] Admin login sets `HttpOnly` session cookies; logout clears them.
- [ ] Mobile token flow: `/auth/token` returns body tokens, refresh
      rotates, body-token logout revokes (401 on reuse).
- [ ] Viewer cannot create projects / deploy / reveal secrets (403).
- [ ] Operator cannot mint exec shells / restore backups / manage users (403).
- [ ] Unauthenticated requests to `/server-hub/api/*` return 401.

## Fleet & monitoring

- [ ] Agent registers, heartbeat shows ONLINE, metrics flow with fresh timestamps.
- [ ] Stale agent (heartbeat stopped) flips to OFFLINE after the timeout.
- [ ] Alert fires on threshold breach and resolves; notification received.
- [ ] `/health/live` 200 during DB outage; `/health/ready` 503 → 200 on recovery.

## Containers & deployments

- [ ] Start/stop/restart a test container with confirmation gating.
- [ ] Stop of `serverhub-postgres` (or own container) refused with 403.
- [ ] Exec shell mint + attach works for admin; viewer log view shows no token.
- [ ] Deploy with outside-root `deployment_path` rejected; valid deploy succeeds.
- [ ] Rollback response carries the redeploy note.

## Backups

- [ ] Backup completes; download is a non-empty zip.
- [ ] Restore overwrites and verifies content; pre-restore snapshot row exists.
- [ ] Restore of FAILED/missing backup rejected (409/404).

## Frontend

- [ ] Dashboard loads; loading, empty, error, and retry states render.
- [ ] SSE reconnect after API restart refreshes stale data.
- [ ] Destructive dialogs name the exact target and consequence.
