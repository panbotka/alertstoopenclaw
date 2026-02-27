package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// NewMux creates the HTTP handler with /webhook and /healthz routes.
func NewMux(queue *AlertQueue, webhookToken string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhook", webhookHandler(queue, webhookToken))
	mux.HandleFunc("GET /healthz", healthzHandler)
	return mux
}

// webhookHandler returns an HTTP handler that validates and enqueues Alertmanager webhook payloads.
func webhookHandler(queue *AlertQueue, webhookToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r, webhookToken) {
			return
		}
		if !checkContentType(w, r) {
			return
		}

		// Limit request body to 1 MB.
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		var payload AlertmanagerPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			slog.Warn("invalid webhook payload", "error", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Only forward firing alerts.
		if payload.Status != "firing" {
			slog.Info("ignoring non-firing alert", "status", payload.Status)
			w.WriteHeader(http.StatusOK)
			return
		}

		if !queue.Enqueue(&payload) {
			slog.Warn("failed to enqueue alert, queue full")
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

// checkAuth validates the bearer token if webhook authentication is configured.
func checkAuth(w http.ResponseWriter, r *http.Request, webhookToken string) bool {
	if webhookToken == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	if auth != "Bearer "+webhookToken {
		//nolint:gosec // G706: structured slog key-value, not string interpolation.
		slog.Warn("unauthorized webhook request", "remote_addr", r.RemoteAddr)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

// checkContentType validates the Content-Type header is application/json when present.
func checkContentType(w http.ResponseWriter, r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return true
	}
	mediaType := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
	if !strings.EqualFold(mediaType, "application/json") {
		//nolint:gosec // G706: structured slog key-value, not string interpolation.
		slog.Warn("unsupported content type", "content_type", ct)
		http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

// healthzHandler responds with a JSON health check status.
func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
