package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Structured JSON logging.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// Load configuration from environment variables.
	listenAddr := envOr("LISTEN_ADDR", ":8080")
	openclawURL := os.Getenv("OPENCLAW_URL")
	openclawToken := os.Getenv("OPENCLAW_TOKEN")
	openclawModel := envOr("OPENCLAW_MODEL", "openclaw:main")
	webhookToken := os.Getenv("WEBHOOK_TOKEN")

	if openclawURL == "" {
		slog.Error("OPENCLAW_URL is required")
		os.Exit(1)
	}
	if openclawToken == "" {
		slog.Error("OPENCLAW_TOKEN is required")
		os.Exit(1)
	}

	slog.Info("starting alertstoopenclaw",
		"listen_addr", listenAddr,
		"openclaw_url", openclawURL,
		"openclaw_model", openclawModel,
		"webhook_auth", webhookToken != "",
	)

	// Create components.
	client := NewOpenClawClient(openclawURL, openclawToken, openclawModel)
	queue := NewAlertQueue(client)
	queue.Start()

	mux := NewMux(queue, webhookToken)
	server := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			stop()
		}
	}()

	slog.Info("server started", "addr", listenAddr)

	<-ctx.Done()
	slog.Info("shutting down")

	// Give in-flight HTTP requests 5 seconds to complete.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)

	// Drain the alert queue.
	queue.Stop()

	slog.Info("shutdown complete")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
