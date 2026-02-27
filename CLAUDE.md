# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o alertstoclaude .
OPENCLAW_URL=http://openclaw:18789 OPENCLAW_TOKEN=<token> ./alertstoclaude
```

There are no external dependencies (stdlib only) and no tests yet.

**Lint:** `golangci-lint run` (available on this system)

**Docker:**
```bash
docker build -t alertstoclaude .
docker run -e OPENCLAW_URL=... -e OPENCLAW_TOKEN=... -p 8080:8080 alertstoclaude
```

## Architecture

Stateless bridge service: receives Grafana Alertmanager webhook POSTs, formats a prompt, and forwards it to an OpenClaw instance (Claude agent) for autonomous investigation/remediation.

```
Grafana Alertmanager --POST /webhook--> [Go App] --POST /v1/chat/completions--> [OpenClaw]
```

**Request flow:** `handler.go` (auth check, parse JSON, filter firing-only) → `queue.go` (buffered channel, single consumer goroutine) → `openclaw.go` (build prompt, POST to OpenClaw with 3-retry exponential backoff).

The queue ensures alerts are forwarded one at a time (sequential processing). Fire-and-forget: the app returns 200 to Grafana immediately after enqueuing, and discards the OpenClaw response body.

## Configuration

All via environment variables — no config files:

| Variable | Required | Default | Description |
|---|---|---|---|
| `LISTEN_ADDR` | No | `:8080` | Server listen address |
| `OPENCLAW_URL` | Yes | — | OpenClaw base URL |
| `OPENCLAW_TOKEN` | Yes | — | Bearer token for OpenClaw API |
| `WEBHOOK_TOKEN` | No | (empty) | If set, inbound webhooks must send this as Bearer token |

## Key Design Decisions

- **Stdlib only** — zero external dependencies (`net/http`, `encoding/json`, `log/slog`)
- **Firing only** — resolved alerts are ignored (return 200, no forwarding)
- **No deduplication** — relies on Alertmanager's grouping/repeat_interval
- **Queue buffer: 100** — alerts are dropped with a warning log if full
- **30s HTTP timeout** per OpenClaw request; retries: 3 attempts with 1s/2s backoff between retries
- **CI** — GitHub Actions workflow builds and pushes multi-arch Docker image to GHCR
