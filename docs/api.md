# API Reference

## POST /webhook

Receives Alertmanager webhook payloads.

### Authentication

If the `WEBHOOK_TOKEN` environment variable is set, requests must include an `Authorization` header:

```
Authorization: Bearer <token>
```

### Request

- **Content-Type:** `application/json` (validated if present)
- **Body:** Alertmanager webhook JSON payload (max 1 MB)

```json
{
  "version": "4",
  "status": "firing",
  "alerts": [
    {
      "status": "firing",
      "labels": { "alertname": "HighCPU", "instance": "server1" },
      "annotations": { "summary": "CPU usage above 90%" },
      "startsAt": "2026-01-01T00:00:00Z",
      "fingerprint": "abc123"
    }
  ],
  "groupLabels": { "alertname": "HighCPU" },
  "commonLabels": { "alertname": "HighCPU" },
  "commonAnnotations": { "summary": "CPU usage above 90%" },
  "externalURL": "http://grafana:3000"
}
```

### Response Codes

| Code | Meaning |
|---|---|
| 200 | Alert enqueued (firing) or acknowledged (resolved/non-firing) |
| 400 | Malformed JSON or body exceeds 1 MB |
| 401 | Missing or invalid bearer token (when `WEBHOOK_TOKEN` is set) |
| 415 | Content-Type header present but not `application/json` |
| 503 | Processing queue is full (Alertmanager will retry) |

### Example

```bash
curl -X POST http://localhost:8080/webhook \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-token" \
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

## GET /healthz

Returns a JSON health check status.

### Response

```json
{"status": "ok"}
```

### Example

```bash
curl http://localhost:8080/healthz
```
