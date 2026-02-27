package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// NewMux creates the HTTP handler with /webhook and /healthz routes.
func NewMux(queue *AlertQueue, webhookToken string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhook", webhookHandler(queue, webhookToken))
	mux.HandleFunc("GET /healthz", healthzHandler)
	return mux
}

func webhookHandler(queue *AlertQueue, webhookToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check auth if configured.
		if webhookToken != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+webhookToken {
				slog.Warn("unauthorized webhook request", "remote_addr", r.RemoteAddr)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

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
		}

		w.WriteHeader(http.StatusOK)
	}
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
