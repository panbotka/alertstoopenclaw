package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBuildPrompt(t *testing.T) {
	payload := &AlertmanagerPayload{
		Version: "4",
		Status:  "firing",
		Alerts: []Alert{
			{
				Status:      "firing",
				Labels:      map[string]string{"alertname": "HighCPU"},
				Annotations: map[string]string{"summary": "CPU high"},
				StartsAt:    "2026-01-01T00:00:00Z",
				Fingerprint: "abc123",
			},
		},
		CommonLabels: map[string]string{"alertname": "HighCPU"},
	}

	prompt, err := buildPrompt(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, `"alertname": "HighCPU"`) {
		t.Error("prompt should contain alert JSON")
	}
	if !strings.Contains(prompt, "Investigate the alert") {
		t.Error("prompt should contain instruction text")
	}
}

func TestForward_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewOpenClawClient(server.URL, "token", "test-model")
	payload := &AlertmanagerPayload{
		Status:       "firing",
		Alerts:       []Alert{{Status: "firing", Labels: map[string]string{"alertname": "Test"}}},
		CommonLabels: map[string]string{"alertname": "Test"},
	}

	if err := client.Forward(context.Background(), payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForward_Retry(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewOpenClawClient(server.URL, "token", "test-model")
	payload := &AlertmanagerPayload{
		Status:       "firing",
		Alerts:       []Alert{{Status: "firing", Labels: map[string]string{"alertname": "Test"}}},
		CommonLabels: map[string]string{"alertname": "Test"},
	}

	if err := client.Forward(context.Background(), payload); err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}

	if got := count.Load(); got != 3 {
		t.Fatalf("expected 3 requests, got %d", got)
	}
}

func TestForward_AllRetriesFail(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewOpenClawClient(server.URL, "token", "test-model")
	payload := &AlertmanagerPayload{
		Status:       "firing",
		Alerts:       []Alert{{Status: "firing", Labels: map[string]string{"alertname": "Test"}}},
		CommonLabels: map[string]string{"alertname": "Test"},
	}

	err := client.Forward(context.Background(), payload)
	if err == nil {
		t.Fatal("expected error after all retries fail")
	}
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("unexpected error message: %v", err)
	}

	if got := count.Load(); got != 3 {
		t.Fatalf("expected 3 requests, got %d", got)
	}
}

func TestForward_ContextCancelled(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewOpenClawClient(server.URL, "token", "test-model")
	payload := &AlertmanagerPayload{
		Status:       "firing",
		Alerts:       []Alert{{Status: "firing", Labels: map[string]string{"alertname": "Test"}}},
		CommonLabels: map[string]string{"alertname": "Test"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err := client.Forward(ctx, payload)
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}

	// Should have made at most 1 request (the first attempt before backoff).
	if got := count.Load(); got > 1 {
		t.Fatalf("expected at most 1 request with cancelled context, got %d", got)
	}
}
