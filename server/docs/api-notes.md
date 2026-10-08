# ServerHub API notes (Phase 1.5)

Deferred work and known inconsistencies. Nothing here changes behavior —
each item is scoped for a future API-versioning effort, NOT for ad-hoc fixes.

## 1. Error response standardization (deferred)

Current APIs return flat errors:

```json
{ "error": "message" }
```

Observed inconsistencies (kept for compatibility — web/mobile parse this shape):

- Some handlers return 404 with `{"error": ...}`, others return 200 + `{"ok": true}` for no-ops.
- Validation failures are usually 400, but a few older paths return 500 for bad input.
- Success envelopes vary (`{"ok": true}` vs resource payloads vs `{"deleted": n}`).

Future format (introduce together with API versioning, e.g. `/api/v2`):

```json
{
  "error": {
    "code": "SERVER_NOT_FOUND",
    "message": "Server does not exist",
    "requestId": "..."
  }
}
```

Do NOT migrate piecemeal: mixed formats are worse than one consistent flat format.

## 2. Notification history (partial gap)

Answerable today:

- *What/when/channel?* — `in_app_notifications` rows (per user, linked `alert_id`/`server_id`).
- *Did email fail?* — `app_logs` (`source: system`, `notify: email failed: …`).
- *Did email succeed?* — `app_logs` success marker (`notify: email sent (<event>, N recipient(s))`, counts only, no addresses).

Not answerable (Phase 2/3 scope): per-recipient delivery receipts, retry
history, Telegram message IDs. No schema changes were made for this in
Phase 1.5; the tables above must remain the source of truth until then.

## 3. Notification flood semantics (current, by design)

- Internal alerts: notify only on TRIGGERED transition (`fireAlert`
  no-ops while a TRIGGERED row exists for `(server_id, condition)`).
- Alertmanager webhook: same, keyed on `fingerprint`.
- Flapping (TRIGGERED→RESOLVED→TRIGGERED) notifies once per cycle.
- Cooldowns, digest batching, and per-rule throttling belong to the Phase 2
  notification rules engine — deliberately not implemented here.
