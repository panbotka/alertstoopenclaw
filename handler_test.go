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

func testPayload(status string) string {
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
	b, _ := json.Marshal(p)
	return string(b)
}

func TestFiringAlert(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(testPayload("firing")))
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
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(testPayload("resolved")))
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

func TestUnauthorized(t *testing.T) {
	client := NewOpenClawClient("http://localhost", "token", "model")
	queue := NewAlertQueue(client)
	queue.Start()
	defer queue.Stop()

	mux := NewMux(queue, "secret-token")
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(testPayload("firing")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthValid(t *testing.T) {
	client := NewOpenClawClient("http://localhost", "token", "model")
	queue := NewAlertQueue(client)
	queue.Start()
	defer queue.Stop()

	mux := NewMux(queue, "secret-token")
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(testPayload("firing")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestBadJSON(t *testing.T) {
	client := NewOpenClawClient("http://localhost", "token", "model")
	queue := NewAlertQueue(client)
	queue.Start()
	defer queue.Stop()

	mux := NewMux(queue, "")
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestOversizedBody(t *testing.T) {
	client := NewOpenClawClient("http://localhost", "token", "model")
	queue := NewAlertQueue(client)
	queue.Start()
	defer queue.Stop()

	mux := NewMux(queue, "")
	// Create a body larger than 1 MB.
	bigBody := `{"status":"firing","alerts":[` + strings.Repeat(`{"status":"firing","labels":{"x":"`+strings.Repeat("A", 1000)+`"}},`, 1100) + `]}` //nolint:lll
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWrongContentType(t *testing.T) {
	client := NewOpenClawClient("http://localhost", "token", "model")
	queue := NewAlertQueue(client)
	queue.Start()
	defer queue.Stop()

	mux := NewMux(queue, "")
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(testPayload("firing")))
	req.Header.Set("Content-Type", "text/xml")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", w.Code)
	}
}

func TestQueueFull(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(testPayload("firing")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHealthz(t *testing.T) {
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
