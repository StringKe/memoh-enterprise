package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/memohai/memoh/internal/connectapi/gen/memoh/browser/v1/browserv1connect"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/runner"
)

const (
	defaultAgentRunnerAddr    = ":26813"
	defaultServerAddr         = "http://127.0.0.1:26810"
	defaultBrowserGatewayAddr = "http://127.0.0.1:26812"
)

func run(parent context.Context) error {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := os.Getenv("MEMOH_AGENT_RUNNER_ADDR")
	if addr == "" {
		addr = defaultAgentRunnerAddr
	}
	serverAddr := os.Getenv("MEMOH_SERVER_ADDR")
	if serverAddr == "" {
		serverAddr = defaultServerAddr
	}
	support := runnerv1connect.NewRunnerSupportServiceClient(http.DefaultClient, serverAddr)
	browserAddr := os.Getenv("MEMOH_BROWSER_GATEWAY_ADDR")
	if browserAddr == "" {
		browserAddr = defaultBrowserGatewayAddr
	}
	browser := runner.NewBrowserClient(browserv1connect.NewBrowserServiceClient(http.DefaultClient, browserAddr))

	mux := http.NewServeMux()
	path, handler := runnerv1connect.NewRunnerServiceHandler(runner.NewService(runner.ServiceDeps{
		SupportClient: support,
		ContextClient: runner.NewContextClient(support),
		Workspace:     runner.NewWorkspaceClient(support, http.DefaultClient),
		Provider:      runner.NewProviderClient(support),
		Memory:        runner.NewMemoryClient(support),
		ToolApproval:  runner.NewToolApprovalClient(support),
		Browser:       browser,
	}))
	mux.Handle(path, handler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           h2c.NewHandler(mux, &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.L.InfoContext(ctx, "agent runner listening", slog.String("addr", addr))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
