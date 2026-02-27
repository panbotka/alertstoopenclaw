package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// OpenClawClient sends alert prompts to an OpenClaw instance.
type OpenClawClient struct {
	baseURL string
	token   string
	model   string
	client  *http.Client
}

// NewOpenClawClient creates a client with a 30-second timeout.
func NewOpenClawClient(baseURL, token, model string) *OpenClawClient {
	return &OpenClawClient{
		baseURL: baseURL,
		token:   token,
		model:   model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// chatRequest is the request body for the OpenClaw chat completions API.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// buildPrompt creates the structured prompt from an Alertmanager payload.
func buildPrompt(payload *AlertmanagerPayload) (string, error) {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	prompt := fmt.Sprintf(`You received the following Grafana Alertmanager webhook payload:

`+"```json\n%s\n```"+`

Investigate the alert(s) above. Try to identify the root cause and resolve the issue if possible.
If you cannot resolve it, provide a detailed diagnosis and suggest remediation steps.
Report your findings and the current status (resolved, in-progress, or needs-manual-intervention).`, raw)

	return prompt, nil
}

// Forward sends the alert payload to OpenClaw with up to 3 retries and exponential backoff.
func (c *OpenClawClient) Forward(ctx context.Context, payload *AlertmanagerPayload) error {
	prompt, err := buildPrompt(payload)
	if err != nil {
		return fmt.Errorf("build prompt: %w", err)
	}

	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/v1/chat/completions"

	var lastErr error
	for attempt := range 3 {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second // 1s, 2s
			slog.Info("retrying openclaw request", "attempt", attempt+1, "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during backoff: %w", ctx.Err())
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.token)

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			slog.Warn("openclaw request error", "attempt", attempt+1, "error", err)
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return nil
		}

		// Read up to 512 bytes of the error response body for logging.
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()

		lastErr = fmt.Errorf("openclaw returned status %d", resp.StatusCode)
		slog.Warn("openclaw non-2xx response", "attempt", attempt+1, "status", resp.StatusCode, "body", string(errBody))
	}

	return fmt.Errorf("openclaw request failed after 3 attempts: %w", lastErr)
}
