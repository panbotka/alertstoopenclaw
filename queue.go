package main

import (
	"context"
	"log/slog"
	"sync"
)

// AlertQueue processes alert payloads sequentially via a single consumer goroutine.
type AlertQueue struct {
	ch       chan *AlertmanagerPayload
	client   *OpenClawClient
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
	stopOnce sync.Once
}

// NewAlertQueue creates a buffered queue with capacity 100.
func NewAlertQueue(client *OpenClawClient) *AlertQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &AlertQueue{
		ch:     make(chan *AlertmanagerPayload, 100),
		client: client,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start launches the consumer goroutine.
func (q *AlertQueue) Start() {
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		for payload := range q.ch {
			alertname := payload.CommonLabels["alertname"]
			slog.Info("processing alert", "alertname", alertname, "status", payload.Status, "alert_count", len(payload.Alerts))

			if err := q.client.Forward(q.ctx, payload); err != nil {
				slog.Error("failed to forward alert to openclaw", "alertname", alertname, "error", err)
			} else {
				slog.Info("alert forwarded to openclaw", "alertname", alertname)
			}
		}
		slog.Info("alert queue consumer stopped")
	}()
}

// Enqueue adds a payload to the queue. Returns false if the queue is full.
func (q *AlertQueue) Enqueue(payload *AlertmanagerPayload) bool {
	select {
	case q.ch <- payload:
		return true
	default:
		slog.Warn("alert queue full, dropping alert", "alertname", payload.CommonLabels["alertname"])
		return false
	}
}

// Stop cancels in-flight operations, closes the channel, and waits for the consumer to drain.
func (q *AlertQueue) Stop() {
	q.stopOnce.Do(func() {
		q.cancel()
		slog.Info("draining alert queue", "remaining", len(q.ch))
		close(q.ch)
	})
	q.wg.Wait()
}
