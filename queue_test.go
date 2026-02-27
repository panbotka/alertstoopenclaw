package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAlertQueue_StartStop(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewOpenClawClient(server.URL, "token", "model")
	queue := NewAlertQueue(client)
	queue.Start()
	queue.Stop()

	// Calling Stop again should not panic (sync.Once).
	queue.Stop()
}

func TestAlertQueue_Enqueue(t *testing.T) {
	t.Parallel()

	called := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		select {
		case called <- struct{}{}:
		default:
		}
	}))
	defer server.Close()

	client := NewOpenClawClient(server.URL, "token", "model")
	queue := NewAlertQueue(client)
	queue.Start()
	defer queue.Stop()

	payload := &AlertmanagerPayload{
		Status:       "firing",
		Alerts:       []Alert{{Status: "firing", Labels: map[string]string{"alertname": "Test"}}},
		CommonLabels: map[string]string{"alertname": "Test"},
	}

	if !queue.Enqueue(payload) {
		t.Fatal("expected enqueue to succeed")
	}

	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for alert to be processed")
	}
}
