# Notification Rules Engine (Phase 2)

## Architecture

```text
Alert transition (internal loop / Alertmanager webhook)
        │  (existing in-app + SSE path runs unchanged first)
        ▼
NotificationEvent { Type, ServerID, Severity, Condition, Value,
                    Threshold, Message, Fingerprint, AlertID }
        │
        ▼
rules.Engine.Evaluate  (no-op when NOTIFICATION_RULES_ENABLED=false)
        │  1. load enabled rules for the event type, id ASC
        │  2. severity match (firing events only)
        │  3. structured condition match (firing events only)
        │  4. claim (rule_id, fingerprint) state row (SELECT … FOR UPDATE)
        │     - recovery without preceding firing → silent bookkeeping
        │     - firing inside cooldown window → suppress
        │  5. dispatch EMAIL and/or IN_APP via existing notify package
        │  (one rule failing never blocks the others)
        ▼
Existing dispatcher (notify.SendEmailTo / notify.NotifyUsers + SMTP)
```

Clients never touch PostgreSQL or Prometheus directly; rules reference
`NotificationGroup` rows; Prometheus/Alertmanager remain optional.

## Producers → event types

| Producer | Firing event | Recovery event |
|---|---|---|
| Offline loop (`alerting`) | `AGENT_OFFLINE` (CRITICAL) | `AGENT_ONLINE` (on actual resolve) |
| Threshold loop (`alerting`) | `SERVER_ALERT` (condition/value/threshold) | `SERVER_ALERT_RESOLVED` |
| Alertmanager webhook (`handlers`) | `SERVER_ALERT` (alertname/fingerprint) | `SERVER_ALERT_RESOLVED` |

Health/healthcheck SSE-only signals are **not** producers (future sources).

## Rule model

`notification_rules`: name (unique), description, enabled, event_type,
severity matcher ("" = any), condition_json (optional structured
`{field, operator, value}`), notification_group_id, channels (`EMAIL,IN_APP`
subset), cooldown_seconds (0–2592000), notify_on_recovery, created_by /
updated_by, timestamps. Recovery matching keys on incident identity
(rule + fingerprint + prior firing), never on re-evaluated thresholds.

## Endpoints (`/server-hub/api`, existing flat `{"error"}` format)

| Method | Path | RBAC | Notes |
|---|---|---|---|
| GET | `/notification-rules` | viewer+ | includes `groupName` join |
| GET | `/notification-rules/:id` | viewer+ | 404 unknown |
| POST | `/notification-rules` | operator+ | 400 validation, 409 name clash |
| PATCH | `/notification-rules/:id` | operator+ | partial merge, 404/409 |
| DELETE | `/notification-rules/:id` | admin | 404 unknown; state rows cleaned |

Group deletion with referencing rules → **409** (restrict; disable/delete
rules first — no silent fallback, no redirection).

## Operators / channels / RBAC / cooldown / recovery / dedupe

- Operators: `eq neq gt gte lt lte contains in`; fields `severity condition
  value serverId`; strict schema validation, 400 otherwise.
- Channels: `EMAIL` (SMTP to rule group members via existing delivery),
  `IN_APP` (per-user rows with alert link). No other channels exist.
- RBAC: viewer read, operator create/update, admin delete (same middleware
  as notification groups; permission boundaries covered live + by route table).
- Cooldown: per (rule, fingerprint), row-locked claim; concurrent
  evaluations deliver exactly once (tested with 10 goroutines).
- Dedupe: internal transitions already fire once per TRIGGERED lifecycle;
  webhook duplicates collapse on fingerprint upstream; engine adds
  per-rule cooldown identity on top.
- Recovery: only when `notifyOnRecovery` AND a firing notification preceded
  it; repeats are silent bookkeeping.
- Dual configuration is deterministic: if both a `SERVER_ALERT` rule (with
  recovery on) and a `SERVER_ALERT_RESOLVED` rule exist, exactly one recovery
  notification is sent. Rule state is per (rule, fingerprint), so the
  resolved-type rule — which never recorded a firing — suppresses itself
  ("recovery without preceding firing") regardless of evaluation order.
  Prefer the toggle on the firing rule; dedicated resolved-type rules are
  allowed but redundant.

## Delivery semantics

At-most-once per cooldown window is favored: state commits before SMTP, so
a crash between commit and send may skip (never duplicate) a notification.
SMTP is never performed inside the state transaction. Failures are logged
(`notify: rule …`) with rule/event/channel context and no secrets.

## Rollout

1. Deploy schema (auto-created on boot, idempotent) + code.
2. `NOTIFICATION_RULES_ENABLED` defaults **false** — legacy behavior bit-for-bit.
3. Create rules disabled, verify, enable the flag, monitor.
4. Never auto-migrate legacy behavior into rules.
