# ServerHub

A self-hosted server monitoring and management platform.

## Architecture

```
Web App (React + Vite)
Mobile App (Expo React Native)
        ↓
  Express-style API (Go + Gin)
        ↓
   PostgreSQL Database
        ↓
  ┌─────────────────────┐
  │  Server Agent       │  (Go binary, per managed server)
  └─────────────────────┘
        ↓
  Managed Linux Servers

  ┌─────────────────────┐     ┌─────────────────────┐
  │  Prometheus :9090   │     │  Alertmanager :9093  │
  │  node_exporter      │     │  webhook → ServerHub │
  │  cAdvisor           │     └─────────────────────┘
  └─────────────────────┘
        ↑ scrape
  node-exporter · cAdvisor · other exporters
```

ServerHub acts as the **authenticated API gateway** for both Prometheus and
Alertmanager. The browser never communicates with those systems directly.

## Features

- **Multi-server management** — register and monitor remote Linux servers
- **Server Agent** — lightweight Node.js agent reports heartbeat + metrics
- **Real-time monitoring** — SSE-based live updates (CPU, RAM, disk, network)
- **Historical metrics** — per-server time-series (1h / 6h / 24h / 7d / 30d)
- **Prometheus integration** — authenticated proxy for PromQL queries, targets, rules, and range metrics
- **Alertmanager integration** — alerts, silences (create/delete), webhook receiver → SSE
- **Monitoring dashboard** — CPU/memory/disk overview, active alerts, target health, metrics charts
- **Alert engine** — threshold alerts (CPU/RAM/disk) + offline detection, TRIGGERED→RESOLVED state machine
- **Health checks** — HTTP, TCP, and Ping probes with result history
- **In-app notifications** — per-user notification center
- **Audit log** — immutable record of all significant actions
- **Docker management** — container lifecycle, logs, stats (local Docker)
- **Project management** — Docker Compose project tracking and deployment
- **RBAC** — viewer / operator / admin roles enforced on every API endpoint
- **Server groups** — organise servers by environment (Production, Development, etc.)
- **Agent tokens** — secure hashed token authentication for agents
- **Secrets management** — AES-GCM encrypted project secrets
- **Database browser** — auto-detect and browse Postgres/MySQL/Redis/MongoDB
- **Backups** — project snapshot backups
- **Notification channels** — Telegram and email alerts

## Server Agent

The agent runs on each managed Linux server and reports metrics to the ServerHub API.
It is a single Go binary — no runtime needed on the monitored machine.

### Install

```bash
git clone https://github.com/Sri20112001/server-hub.git /opt/serverhub-agent
cd /opt/serverhub-agent/agent
go build -o serverhub-agent ./cmd/agent
```

### Run

```bash
AGENT_TOKEN=<token-from-web-ui> \
SERVERHUB_URL=http://<serverhub-host>:4000 \
./serverhub-agent
```

Generate a token: **Web UI → Servers → select server → Tokens tab → Generate**

### Systemd service

```ini
[Unit]
Description=ServerHub Agent
After=network.target

[Service]
WorkingDirectory=/opt/serverhub-agent/agent
ExecStart=/opt/serverhub-agent/agent/serverhub-agent
Restart=always
Environment=AGENT_TOKEN=<token>
Environment=SERVERHUB_URL=http://<host>:4000

[Install]
WantedBy=multi-user.target
```

## Environment Variables

### Backend (`server/.env`)

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL DSN |
| `JWT_SECRET` | Yes | Min 32 chars |
| `SERVERHUB_ENCRYPTION_KEY` | Yes | 64-char hex (32 bytes) |
| `ADMIN_PASSWORD` | Yes | Initial admin password |
| `ADMIN_USERNAME` | No | Default: `admin` |
| `PORT` | No | Default: `4000` |
| `FRONTEND_URL` | No | CORS allowed origins |
| `ALERT_CPU_PCT` | No | CPU alert threshold (default: 85) |
| `ALERT_RAM_PCT` | No | RAM alert threshold (default: 90) |
| `ALERT_DISK_PCT` | No | Disk alert threshold (default: 80) |
| `HEALTH_CHECK_INTERVAL_SEC` | No | Health check interval (default: 60) |
| `PROMETHEUS_URL` | No | Prometheus base URL (e.g. `http://prometheus:9090`) |
| `PROMETHEUS_TIMEOUT_SEC` | No | Prometheus request timeout (default: 10) |
| `ALERTMANAGER_URL` | No | Alertmanager base URL (e.g. `http://alertmanager:9093`) |
| `ALERTMANAGER_TIMEOUT_SEC` | No | Alertmanager request timeout (default: 10) |
| `ALERTMANAGER_WEBHOOK_SECRET` | No | Shared secret for the Alertmanager webhook endpoint |

### Agent

| Variable | Required | Description |
|---|---|---|
| `AGENT_TOKEN` | Yes | Token from ServerHub web UI |
| `SERVERHUB_URL` | Yes | Base URL of the API |
| `METRICS_INTERVAL` | No | Seconds between metric reports (default: 60) |
| `HEARTBEAT_INTERVAL` | No | Seconds between heartbeats (default: 30) |

## Development

### Backend

```bash
cd server
cp .env.example .env   # fill in values
docker compose up -d postgres
go run ./cmd/server
```

### Web frontend

```bash
cd client
npm install
npm run dev
```

### Mobile app

```bash
cd mobile
npm install
npx expo start
```

### Agent

```bash
cd agent
go build -o serverhub-agent ./cmd/agent
AGENT_TOKEN=<token> SERVERHUB_URL=http://localhost:4000 ./serverhub-agent
```

### Docker (full stack)

Includes Prometheus, Alertmanager, node-exporter, and cAdvisor:

```bash
cd server
docker compose up --build
```

Prometheus scrapes node-exporter and cAdvisor automatically.
Alertmanager sends webhooks to `http://serverhub:4000/server-hub/api/webhooks/alertmanager`.

To enable the Alertmanager webhook shared secret:
1. Set `ALERTMANAGER_WEBHOOK_SECRET=your-secret` in `server/.env`
2. Uncomment the `http_config` block in `server/alertmanager.yml` and set the same value

## Prometheus Setup

Required exporters for full monitoring coverage:

| Exporter | Port | Purpose |
|---|---|---|
| `node_exporter` | 9100 | Host CPU, memory, disk, network |
| `cAdvisor` | 8080 | Container resource usage |

Add additional scrape targets to `server/prometheus.yml`.

## Alertmanager Setup

Alertmanager is configured in `server/alertmanager.yml`. The default config
routes all alerts to the ServerHub webhook receiver.

Webhook URL: `http://serverhub:4000/server-hub/api/webhooks/alertmanager`

When an alert fires or resolves, Alertmanager POSTs to this endpoint.
ServerHub validates the payload, logs it, and publishes SSE events
(`monitoring.alert.firing` / `monitoring.alert.resolved`) to all connected clients.

## API

```
# Auth
POST   /server-hub/api/auth/login
POST   /server-hub/api/auth/token        # mobile
POST   /server-hub/api/auth/refresh
POST   /server-hub/api/auth/logout
GET    /server-hub/api/auth/me

# Managed Servers
GET    /server-hub/api/servers
POST   /server-hub/api/servers
GET    /server-hub/api/servers/:id
PATCH  /server-hub/api/servers/:id
DELETE /server-hub/api/servers/:id
GET    /server-hub/api/servers/:id/metrics?range=1h|6h|24h|7d|30d
GET    /server-hub/api/servers/:id/metrics/latest
GET    /server-hub/api/servers/:id/tokens
POST   /server-hub/api/servers/:id/tokens
DELETE /server-hub/api/servers/:id/tokens/:tokenId

# Agent (token auth)
POST   /server-hub/api/agent/heartbeat
POST   /server-hub/api/agent/metrics

# Server Groups
GET    /server-hub/api/server-groups
POST   /server-hub/api/server-groups
DELETE /server-hub/api/server-groups/:id

# Alerts
GET    /server-hub/api/alerts?status=TRIGGERED|RESOLVED
PATCH  /server-hub/api/alerts/:id/resolve

# Health Checks
GET    /server-hub/api/health-checks
POST   /server-hub/api/health-checks
PATCH  /server-hub/api/health-checks/:id
DELETE /server-hub/api/health-checks/:id
GET    /server-hub/api/health-checks/:id/results

# Notifications
GET    /server-hub/api/notifications
PATCH  /server-hub/api/notifications/:id/read
POST   /server-hub/api/notifications/read-all

# Audit
GET    /server-hub/api/audit?limit=100

# Monitoring — Prometheus proxy (viewer+)
GET    /server-hub/api/monitoring/overview
GET    /server-hub/api/monitoring/metrics?metric=cpu|memory|disk|networkRx|networkTx&range=1h|6h|24h|7d
GET    /server-hub/api/monitoring/prometheus/status
GET    /server-hub/api/monitoring/prometheus/targets
GET    /server-hub/api/monitoring/prometheus/rules
GET    /server-hub/api/monitoring/prometheus/query?query=<promql>          # admin only
GET    /server-hub/api/monitoring/prometheus/query-range?query=...         # admin only

# Monitoring — Alertmanager proxy (viewer+)
GET    /server-hub/api/monitoring/alertmanager/status
GET    /server-hub/api/monitoring/alerts
GET    /server-hub/api/monitoring/silences
POST   /server-hub/api/monitoring/silences                                 # operator+
DELETE /server-hub/api/monitoring/silences/:id                             # operator+

# Webhooks
POST   /server-hub/api/webhooks/github
POST   /server-hub/api/webhooks/alertmanager

# Real-time events (SSE)
GET    /server-hub/api/events
# Event types include:
#   monitoring.alert.firing
#   monitoring.alert.resolved
```
