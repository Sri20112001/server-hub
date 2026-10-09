# Centralized notification delivery

All EMAIL and TELEGRAM messages flow through the delivery pipeline
(`server/internal/delivery`). IN_APP stays direct (cheap in-DB rows).
There is exactly one policy enforcement point — no per-channel
spam-control logic exists anywhere else.

## How an alert becomes a message

1. **Producers** (rules engine, legacy `notify.Send`, deploy/backup/project
   failure paths) build a stable incident key
   (`rule:<id>:<fingerprint>` or `legacy:<event>:<title>`) and call
   `delivery.Dispatch`.
2. **Dispatch** (one transaction: state claim + enqueue are atomic) applies,
   in order: emergency pause → maintenance windows → group quiet hours →
   per-incident dedupe/cooldown/repeats → digest batching → outbox enqueue.
   Anything not sent is recorded with a reason (`cooldown`, `quiet-hours`,
   `rate-limited`, …) — suppression is observable, never silent.
3. **Worker** (`delivery.StartLoop`, 15s) claims due rows with
   `FOR UPDATE SKIP LOCKED` (multi-instance safe), then per row: circuit
   breaker → hourly rate budget → provider send → bounded retry
   (5 attempts, exponential backoff + jitter) or terminal failure.
   Throttled rows defer and coalesce into a per-channel overflow digest.
4. **Digest sweeper** flushes due batches as one summary per batch.

Crash semantics favor at-most-once per enqueue (a crash between state
commit and worker pickup can skip, never duplicate); retries favor
at-least-once per queued row. Stale `sending` rows requeue on every boot.

## Safe defaults

| Severity | First notice | Repeats | Recovery |
|---|---|---|---|
| CRITICAL | immediate | every 15 min, max 3/cycle | once, when enabled |
| WARNING | immediate | every 60 min, max 3/cycle | once, when enabled |
| INFO | immediate (21600s cooldown) | rare | once, when enabled |

Rules override cooldown/repeats/digest per incident; INFO is digest-first
by convention (set the rule to `digest` mode). A resolved-then-retriggered
alert starts a new cycle. Recovery with no preceding firing notification
is muted.

## Configuration

Policy lives in the `notification_policy` singleton (PUT
`/server-hub/api/notification-policy`, admin-only, audit-logged) or the
mobile Notification Settings screen:

- `cooldownCriticalSec/WarningSec/InfoSec`, `repeatIntervalSec`,
  `maxRepeats`, `notifyOnRecovery`
- `emailPerHour`, `tgPerHour` (overflow → summary digest, never silent drop)
- `emergencyPause` + `pauseUntil` + `pauseReason` (pauses sending only;
  evaluation and monitoring continue)
- Quiet hours live on notification groups (`quietStart/End` HH:MM,
  `quietTZ` IANA, `quietAllowCritical`); maintenance windows
  (`all` or `server:<id>`) suppress non-critical delivery while active.

No new environment variables. The Phase 2 rules engine stays behind
`NOTIFICATION_RULES_ENABLED` (rule matching only); the delivery pipeline
itself always runs — legacy callers (`notify.Send`) and rule deliveries
both enter it. Channel credentials stay in `app_settings` (encrypted) and
are never returned by any API.

## Troubleshooting

- "Alert fired but no message": check `GET /notification-deliveries`
  (filter `status=deferred|failed`) and the incident's suppress reason;
  check pause/maintenance/quiet-hours state and channel budgets.
- "Message repeated": check rule `cooldownSeconds` vs policy cooldowns and
  `maxRepeats`; repeats reset only on resolution.
- "Telegram/Email failing": check provider circuit state
  (`delivery_provider_state`) and per-attempt errors in
  `delivery_attempts`; the breaker opens after 5 consecutive failures for
  5 minutes.
- "Digest never arrives": digest rules flush when the window ends; the
  worker logs each flush with its event count.

## Rollback

The pipeline is additive: rules engine falls back to direct sends when
unwired, and legacy callers keep their toggles. To disable, stop the
worker (remove the `delivery.StartLoop` call) — already-enqueued rows sit
in `delivery_outbox` as `pending` without sending. DB tables are plain
migrations; dropping them restores the pre-pipeline schema (loses
history, keeps audit).
