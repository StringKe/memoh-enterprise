package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/memohai/memoh/internal/integrations"
	"github.com/memohai/memoh/internal/logger"
)

const defaultIntegrationGatewayAddr = ":26815"

func run(parent context.Context) error {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := os.Getenv("MEMOH_INTEGRATION_GATEWAY_ADDR")
	if addr == "" {
		addr = defaultIntegrationGatewayAddr
	}
	serverURL := os.Getenv("MEMOH_SERVER_INTERNAL_URL")
	if serverURL == "" {
		return errors.New("MEMOH_SERVER_INTERNAL_URL is required")
	}

	backend := integrations.NewGatewayClient(integrations.GatewayClientOptions{
		BaseURL:      serverURL,
		ServiceToken: os.Getenv("MEMOH_SERVICE_TOKEN"),
	})
	handler := integrations.NewGatewayWebSocketHandler(logger.L, backend)
	mux := http.NewServeMux()
	mux.Handle(integrations.WebSocketPath, handler.HTTPHandler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.L.InfoContext(ctx, "integration gateway listening", slog.String("addr", addr))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
