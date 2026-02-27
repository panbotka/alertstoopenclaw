# Architecture

alertstoopenclaw is a stateless bridge service that receives Grafana Alertmanager webhook POSTs and forwards them to an OpenClaw instance (Claude agent) for autonomous investigation and remediation.

## System Diagram

```
                         POST /webhook                    POST /v1/chat/completions
┌─────────────────────┐ ──────────────> ┌───────────────┐ ──────────────────────────> ┌──────────┐
│ Grafana Alertmanager│                 │alertstoopenclaw│                             │ OpenClaw │
└─────────────────────┘ <────────────── └───────────────┘ <────────────────────────── └──────────┘
                          200 OK                              200 OK (response discarded)
```

## File Responsibilities

| File | Responsibility |
|---|---|
| `main.go` | Entry point — loads config from environment, wires components, runs HTTP server with graceful shutdown |
| `handler.go` | HTTP routing (`/webhook`, `/healthz`), request validation (auth, Content-Type, body size), JSON parsing |
| `queue.go` | Buffered channel (cap 100) with single consumer goroutine, context-aware start/stop |
| `openclaw.go` | Builds structured prompt from alert payload, sends to OpenClaw API with 3-retry exponential backoff |
| `alertmanager.go` | Package doc comment and data types (`AlertmanagerPayload`, `Alert`) |

## Data Flow

1. Grafana Alertmanager sends an HTTP POST to `/webhook` when alerts fire or resolve.
2. `handler.go` validates the bearer token (if configured), checks Content-Type, enforces the 1 MB body limit, and parses the JSON payload.
3. Resolved alerts are acknowledged with 200 and discarded. Firing alerts are placed on the buffered channel.
4. The single consumer goroutine in `queue.go` reads payloads sequentially and calls `openclaw.go:Forward`.
5. `Forward` marshals a chat completions request containing the raw alert JSON and instruction text, then POSTs it to OpenClaw with up to 3 attempts (1s, 2s backoff).
6. The HTTP handler returns 200 immediately after enqueuing (fire-and-forget). The OpenClaw response body is discarded.

## Key Design Decisions

| Decision | Rationale |
|---|---|
| Stdlib only | Zero external dependencies — simplifies builds, reduces supply chain risk |
| Firing only | Resolved alerts need no action; avoids unnecessary OpenClaw invocations |
| Sequential queue | Prevents overloading OpenClaw with concurrent investigations |
| 100-item buffer | Provides burst tolerance; returns 503 when full so Alertmanager retries |
| Fire-and-forget | Decouples webhook response time from OpenClaw processing time |
| 30s HTTP timeout | Prevents hung connections to OpenClaw from blocking the queue |
| Context propagation | Shutdown cancels in-flight requests and retry backoffs cleanly |
