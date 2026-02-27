# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o alertstoopenclaw .
OPENCLAW_URL=http://openclaw:18789 OPENCLAW_TOKEN=<token> ./alertstoopenclaw
```

There are no external dependencies (stdlib only).

**Test:** `make test` (runs `go test -v -race -coverprofile=coverage.out ./...` with coverage report)

**Lint:** `make lint` (strict config in `.golangci.yml`: 27 linters including errcheck, govet, staticcheck, gosec, errorlint, cyclop max 10, funlen 60/40, gocognit 15, nestif 3, lll 120)

**Format:** `make fmt` (runs `gofmt -w .`)

**Full check:** `make check` (chains: fmt, vet, lint, test)

**Dev runner:** `./dev.sh` (builds and starts with PID tracking in `.dev.pid`)

**Docker:**
```bash
docker build -t alertstoopenclaw .
docker run -e OPENCLAW_URL=... -e OPENCLAW_TOKEN=... -p 8080:8080 alertstoopenclaw
```

## Architecture

Stateless bridge service: receives Grafana Alertmanager webhook POSTs, formats a prompt, and forwards it to an OpenClaw instance (Claude agent) for autonomous investigation/remediation.

```
Grafana Alertmanager --POST /webhook--> [Go App] --POST /v1/chat/completions--> [OpenClaw]
```

**Request flow:** `handler.go` (auth via `checkAuth`, content-type via `checkContentType`, body size limit, parse JSON, filter firing-only) → `queue.go` (buffered channel, single consumer goroutine, context-aware shutdown) → `openclaw.go` (build prompt, POST to OpenClaw with 3-retry exponential backoff, context cancellation).

The queue ensures alerts are forwarded one at a time (sequential processing). Fire-and-forget: the app returns 200 to Grafana immediately after enqueuing, and discards the OpenClaw response body.

See [docs/architecture.md](docs/architecture.md) for detailed design and [docs/api.md](docs/api.md) for API reference.

## Configuration

All via environment variables — no config files:

| Variable | Required | Default | Description |
|---|---|---|---|
| `LISTEN_ADDR` | No | `:8080` | Server listen address |
| `OPENCLAW_URL` | Yes | — | OpenClaw base URL |
| `OPENCLAW_TOKEN` | Yes | — | Bearer token for OpenClaw API |
| `WEBHOOK_TOKEN` | No | (empty) | If set, inbound webhooks must send this as Bearer token |
| `OPENCLAW_MODEL` | No | `openclaw:main` | Model name sent to OpenClaw API |

## Key Design Decisions

- **Stdlib only** — zero external dependencies (`net/http`, `encoding/json`, `log/slog`)
- **Firing only** — resolved alerts are ignored (return 200, no forwarding)
- **No deduplication** — relies on Alertmanager's grouping/repeat_interval
- **Queue buffer: 100** — returns 503 when full so Alertmanager retries
- **Request hardening** — 1 MB body limit, Content-Type validation, server read/write/idle timeouts
- **30s HTTP timeout** per OpenClaw request; retries: 3 attempts with 1s/2s backoff between retries
- **Context propagation** — shutdown cancels in-flight OpenClaw requests and retry backoffs
- **Non-root Docker** — runs as unprivileged `appuser`
- **CI** — GitHub Actions workflow builds and pushes multi-arch Docker image to GHCR
