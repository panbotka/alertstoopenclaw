package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testPayload(t *testing.T, status string) string {
	t.Helper()
	p := AlertmanagerPayload{
		Version: "4",
		Status:  status,
		Alerts: []Alert{
			{
				Status:      status,
				Labels:      map[string]string{"alertname": "TestAlert", "instance": "server1"},
				Annotations: map[string]string{"summary": "test alert"},
				StartsAt:    "2026-01-01T00:00:00Z",
				Fingerprint: "abc123",
			},
		},
		GroupLabels:       map[string]string{"alertname": "TestAlert"},
		CommonLabels:      map[string]string{"alertname": "TestAlert"},
		CommonAnnotations: map[string]string{"summary": "test alert"},
		ExternalURL:       "http://grafana:3000",
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal test payload: %v", err)
	}
	return string(b)
}

func TestWebhookHandler_validation(t *testing.T) { //nolint:gocognit // table-driven test with subtests.
	t.Parallel()

	tests := []struct {
		name         string
		webhookToken string
		authHeader   string
		contentType  string
		body         string
		wantCode     int
	}{
		{
			name:         "unauthorized when token required but missing",
			webhookToken: "secret-token",
			authHeader:   "",
			contentType:  "application/json",
			body:         `{"status":"firing","alerts":[]}`,
			wantCode:     http.StatusUnauthorized,
		},
		{
			name:         "authorized with valid token",
			webhookToken: "secret-token",
			authHeader:   "Bearer secret-token",
			contentType:  "application/json",
			body:         `{"status":"firing","alerts":[]}`,
			wantCode:     http.StatusOK,
		},
		{
			name:        "bad JSON returns 400",
			contentType: "application/json",
			body:        "{invalid",
			wantCode:    http.StatusBadRequest,
		},
		{
			name:        "wrong content type returns 415",
			contentType: "text/xml",
			body:        `{"status":"firing","alerts":[]}`,
			wantCode:    http.StatusUnsupportedMediaType,
		},
		{
			name:        "oversized body returns 400",
			contentType: "application/json",
			body:        `{"status":"firing","alerts":[` + strings.Repeat(`{"status":"firing","labels":{"x":"`+strings.Repeat("A", 1000)+`"}},`, 1100) + `]}`, //nolint:lll
			wantCode:    http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := NewOpenClawClient("http://localhost", "token", "model")
			queue := NewAlertQueue(client)
			queue.Start()
			defer queue.Stop()

			mux := NewMux(queue, tt.webhookToken)
			req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d", tt.wantCode, w.Code)
			}
		})
	}
}

func TestFiringAlert(t *testing.T) {
	t.Parallel()

	called := make(chan struct{}, 1)
	ocServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		select {
		case called <- struct{}{}:
		default:
		}
	}))
	defer ocServer.Close()

	client := NewOpenClawClient(ocServer.URL, "test-token", "test-model")
	queue := NewAlertQueue(client)
	queue.Start()
	defer queue.Stop()

	mux := NewMux(queue, "")
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(testPayload(t, "firing")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Wait for the queue to process.
	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OpenClaw server to be called")
	}
}

func TestResolvedAlert(t *testing.T) {
	t.Parallel()

	var called bool
	ocServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ocServer.Close()

	client := NewOpenClawClient(ocServer.URL, "test-token", "test-model")
	queue := NewAlertQueue(client)
	queue.Start()
	defer queue.Stop()

	mux := NewMux(queue, "")
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(testPayload(t, "resolved")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if called {
		t.Fatal("expected OpenClaw server NOT to be called for resolved alert")
	}
}

func TestQueueFull(t *testing.T) {
	t.Parallel()

	client := NewOpenClawClient("http://localhost", "token", "model")
	// Create a queue with capacity 1 and don't start the consumer so it stays full.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queue := &AlertQueue{
		ch:     make(chan *AlertmanagerPayload, 1),
		client: client,
		ctx:    ctx,
		cancel: cancel,
	}
	// Fill the queue.
	queue.ch <- &AlertmanagerPayload{}

	mux := NewMux(queue, "")
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(testPayload(t, "firing")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	client := NewOpenClawClient("http://localhost", "token", "model")
	queue := NewAlertQueue(client)
	queue.Start()
	defer queue.Stop()

	mux := NewMux(queue, "")
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", resp["status"])
	}
}
