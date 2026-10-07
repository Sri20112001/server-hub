# ServerHub Agent (Go)

Single-binary monitoring agent for the ServerHub **Go/Gin** backend.
Collects host metrics via gopsutil and reports heartbeat + metrics to:

- `POST /server-hub/api/agent/heartbeat`
- `POST /server-hub/api/agent/metrics`

Auth: `Authorization: Bearer <agent-token>` (hashed server-side, see
`server/internal/middleware/agent.go`).

## Requirements

- Go 1.26+ to build — **nothing** needed on the monitored machine
  (the compiled binary has no runtime dependencies)
- An agent token: ServerHub web UI → Servers → select server → Tokens → Generate

## Run (dev)

```bash
cd agent
cp /dev/null .env  # or copy the example below into .env
AGENT_TOKEN=<your-token> \
SERVERHUB_URL=http://<serverhub-host>:4000 \
go run ./cmd/agent
```

## Environment variables (.env supported)

| Variable             | Required | Default | Description                          |
|----------------------|----------|---------|--------------------------------------|
| `AGENT_TOKEN`        | Yes      | —       | Token from ServerHub web UI          |
| `SERVERHUB_URL`      | Yes      | —       | Base URL of the ServerHub API        |
| `METRICS_INTERVAL`   | No       | 60      | Seconds between metric reports       |
| `HEARTBEAT_INTERVAL` | No       | 30      | Seconds between heartbeats           |

`.env` is auto-loaded from the binary's directory or cwd (real env vars win).

## Build (single binary, no runtime needed)

```bash
cd agent
# Windows service target
go build -o serverhub-agent.exe ./cmd/agent
# Linux managed server
GOOS=linux GOARCH=amd64 go build -o serverhub-agent ./cmd/agent
```

## Install as a service

Linux (`/etc/systemd/system/serverhub-agent.service`):

```ini
[Unit]
Description=ServerHub Agent
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/serverhub-agent
ExecStart=/opt/serverhub-agent/serverhub-agent
Restart=always
RestartSec=10
Environment=AGENT_TOKEN=<your-token>
Environment=SERVERHUB_URL=http://<serverhub-host>:4000

[Install]
WantedBy=multi-user.target
```

Windows: place `serverhub-agent.exe` + `.env` in
`C:\Program Files\ServerHub\Agent\` and register it as a service
(e.g. via NSSM or `sc.exe create`).

```bash
systemctl daemon-reload
systemctl enable --now serverhub-agent
```
