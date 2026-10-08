# ServerHub monitoring (Phase 3A/3B notes)

## Optionality

Prometheus and Alertmanager are **optional integrations**. ServerHub boots
and operates fully on PostgreSQL alone:

- Blank `PROMETHEUS_URL` / `ALERTMANAGER_URL`, or `PROMETHEUS_ENABLED=false` /
  `ALERTMANAGER_ENABLED=false`, disables each integration without clearing
  its URL. Monitoring proxy endpoints then answer `available: false`
  (or `503 not configured`) instead of failing.
- The Alertmanager webhook ingestion path works independently of the
  Alertmanager *client*: deliveries are accepted, fingerprinted, deduped,
  and fanned out even when the outbound client is disabled.

## Access model

Prometheus is **server-side only**:

- Browsers and mobile apps never address `:9090`/`:9093`. All queries go
  through authenticated, RBAC-gated `/server-hub/api/monitoring/*` proxy
  endpoints on `:4000`.
- Viewer endpoints accept only predefined allowlisted queries; arbitrary
  PromQL (`query`, `query-range`) is admin-only.
- Range queries are bounded server-side (`1h|6h|24h|7d` windows, `15s`–window
  steps, 7d max explicit window). Upstream bodies are capped at 4 MiB with a
  10s timeout.

## Topology

- Docker Compose: `PROMETHEUS_URL=http://prometheus:9090`,
  `ALERTMANAGER_URL=http://alertmanager:9093` (same Compose network,
  internal DNS; neither port is published to the host).
- Local `go run`: set either variable to an explicitly configured local
  address (e.g. `http://localhost:9090`). `localhost` is never hard-coded
  into production code paths.

## Storage split

- **Prometheus remains the time-series database** (high-resolution infra
  metrics, 30d TSDB retention in Compose). Raw Prometheus samples are
  never duplicated into PostgreSQL.
- **PostgreSQL remains application state**, including the `server_metrics`
  snapshots (one row/server/minute from the agent).

## server_metrics retention

Snapshots older than `SERVER_METRICS_RETENTION_DAYS` (default 30, the
largest range the UI/API supports: `1h|6h|24h|7d|30d`) are pruned hourly by
an in-process worker (`internal/retention`):

- Strict cutoff semantics: rows **older than** the cutoff are deleted; a
  row exactly at the cutoff is retained.
- Per-server batched deletes (`5000` rows/batch, `120` batches/run max) so
  the existing `(server_id, timestamp)` composite index serves every
  statement — no new index, no table lock, no long transaction.
- Only `server_metrics` rows are ever deleted. Alerts, servers, tokens,
  groups, rules, logs are untouched.
- The worker stops with the server (SIGINT/SIGTERM graceful shutdown) and
  a repeated run is a safe no-op.

## Per-server Prometheus history (Phase 3C)

Two viewer-readable endpoints expose allowlisted Prometheus history for one
managed server. The backend builds every expression; callers supply only
`metric`, `range`, and `step`:

- `GET /server-hub/api/servers/:id/prometheus/metrics?metric=cpu_usage&range=24h&step=1m`
  → `{serverId, metric, range, step, series: [{name, values: [{timestamp, value}]}]}`
- `GET /server-hub/api/servers/:id/prometheus/metrics/latest?metric=cpu_usage`
  → `{serverId, metric, value, timestamp}` (`value`/`timestamp` null when
  Prometheus has no current sample)

Supported logical metrics (all pinned to `{server_id="<id>"}` from the
validated route parameter — never from client input):

| Logical metric | PromQL strategy | Series behavior |
|---|---|---|
| `cpu_usage` | `100 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m]))*100` | single aggregate across cores |
| `memory_usage` | `100*(1 - MemAvailable/MemTotal)` | single host gauge |
| `disk_usage` | same ratio on `mountpoint="/"` only | canonical root filesystem; pseudo-filesystems excluded by the pin |
| `load_1m` | `node_load1` gauge | single host gauge |
| `network_receive` | `sum(rate(receive_bytes[5m]))`, `device!="lo"` | explicit total across non-loopback interfaces |
| `network_transmit` | same for transmit | same as above |

Rules: server must exist first (`404` before any Prometheus call);
unknown metric/range/step → `400`; Prometheus disabled/unavailable →
`503`/`502` (never 500); ranges `1h|6h|24h|7d`, steps `15s`–window;
responses carry no exporter labels (`instance`/`job`/`device` dropped —
identity is the logical metric name). PostgreSQL `server_metrics`
(range ≤30d, agent snapshots) and Prometheus history remain separate
sources with separate endpoints; nothing is copied between them.
