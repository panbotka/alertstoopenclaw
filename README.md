# alertstoclaude

A lightweight Go service that receives [Grafana Alertmanager](https://grafana.com/docs/grafana/latest/alerting/configure-notifications/manage-contact-points/integrations/webhook-notifier/) webhook notifications and forwards them to an [OpenClaw](https://github.com/anthropics/openclaw) instance for autonomous investigation and remediation.

```
Grafana Alertmanager --webhook--> [alertstoclaude :8080] --HTTP--> [OpenClaw :18789]
```

When an alert fires, alertstoclaude formats a structured prompt containing the full alert payload and sends it to OpenClaw's chat completions API. OpenClaw then autonomously investigates the issue, attempts to resolve it, and reports its findings.

## Features

- Receives Alertmanager webhook payloads on `POST /webhook`
- Filters out resolved alerts (only forwards firing)
- Sequential processing queue (one alert at a time)
- Retry with exponential backoff (3 attempts, 1s and 2s between retries)
- Optional bearer token authentication for inbound webhooks
- Structured JSON logging via `log/slog`
- Health check endpoint at `GET /healthz`
- Graceful shutdown with queue draining
- Zero external dependencies (Go stdlib only)

## Quick Start

```bash
go build -o alertstoclaude .

OPENCLAW_URL=http://localhost:18789 \
OPENCLAW_TOKEN=your-token \
./alertstoclaude
```

## Docker

```bash
docker build -t alertstoclaude .

docker run \
  -e OPENCLAW_URL=http://openclaw:18789 \
  -e OPENCLAW_TOKEN=your-token \
  -p 8080:8080 \
  alertstoclaude
```

## Configuration

All configuration is via environment variables.

| Variable | Required | Default | Description |
|---|---|---|---|
| `LISTEN_ADDR` | No | `:8080` | Address and port to listen on |
| `OPENCLAW_URL` | Yes | — | OpenClaw base URL (e.g. `http://openclaw:18789`) |
| `OPENCLAW_TOKEN` | Yes | — | Bearer token for OpenClaw API |
| `WEBHOOK_TOKEN` | No | *(disabled)* | If set, inbound webhooks must include `Authorization: Bearer <token>` |

## Grafana Alertmanager Setup

Add a webhook contact point in Grafana pointing to your alertstoclaude instance:

- **URL:** `http://alertstoclaude:8080/webhook`
- **HTTP Method:** POST

If you set `WEBHOOK_TOKEN`, configure the contact point to send an `Authorization` header with `Bearer <your-token>`.

## Endpoints

### `POST /webhook`

Receives Alertmanager webhook payloads. Returns `200 OK` immediately after enqueuing (or after ignoring non-firing alerts). Returns `401 Unauthorized` if `WEBHOOK_TOKEN` is set and the request lacks a valid bearer token. Returns `400 Bad Request` for malformed JSON.

### `GET /healthz`

Returns `200 OK` with `{"status":"ok"}`.

## How It Works

1. Grafana Alertmanager sends a webhook POST when alerts fire
2. The handler validates auth (if configured), parses the payload, and rejects non-firing alerts
3. Firing alerts are placed on a buffered channel (capacity 100; dropped with a warning if full)
4. A single consumer goroutine reads from the channel and calls the OpenClaw API
5. The prompt includes the raw alert JSON with instructions to investigate, diagnose, and remediate
6. OpenClaw handles all investigation and reporting through its own channels

## Testing

Send a test alert:

```bash
curl -X POST http://localhost:8080/webhook \
  -H "Content-Type: application/json" \
  -d '{
    "version": "4",
    "status": "firing",
    "alerts": [{
      "status": "firing",
      "labels": {"alertname": "HighCPU", "instance": "server1"},
      "annotations": {"summary": "CPU usage above 90%"},
      "startsAt": "2026-01-01T00:00:00Z",
      "fingerprint": "abc123"
    }],
    "groupLabels": {"alertname": "HighCPU"},
    "commonLabels": {"alertname": "HighCPU"},
    "commonAnnotations": {"summary": "CPU usage above 90%"},
    "externalURL": "http://grafana:3000"
  }'
```

Verify the health endpoint:

```bash
curl http://localhost:8080/healthz
```
