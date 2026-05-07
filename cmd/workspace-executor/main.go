package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1/workspacev1connect"
	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/workspace/executorsvc"
)

const (
	defaultSocketPath = "/run/memoh/workspace-executor.sock"
	templateDir       = "/opt/memoh/templates"
)

func initDataDir() {
	if err := os.MkdirAll(executorsvc.DefaultWorkDir, 0o750); err != nil {
		logger.Warn("failed to create data dir", slog.Any("error", err))
		return
	}

	entries, err := os.ReadDir(templateDir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("failed to read template dir", slog.String("dir", templateDir), slog.Any("error", err))
		}
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		dst := filepath.Join(executorsvc.DefaultWorkDir, entry.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(templateDir, entry.Name()))
		if err != nil {
			continue
		}
		if err := os.WriteFile(dst, data, fs.FileMode(0o644)); err != nil { //nolint:gosec // Template names come from os.ReadDir(templateDir).
			logger.Warn("failed to seed template", slog.String("file", entry.Name()), slog.Any("error", err))
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	initDataDir()
	_ = os.Setenv("PATH", os.Getenv("PATH")+":/opt/memoh/toolkit/bin")

	startDisplaySupervisor(ctx)
	go reapZombies()

	network := "unix"
	address := os.Getenv("WORKSPACE_EXECUTOR_SOCKET_PATH")
	if tcpAddr := os.Getenv("WORKSPACE_EXECUTOR_TCP_ADDR"); tcpAddr != "" {
		network = "tcp"
		address = tcpAddr
	}
	if address == "" {
		address = defaultSocketPath
	}
	if network == "unix" {
		_ = os.Remove(filepath.Clean(address)) //nolint:gosec // G703: address is configured by deployment.
	}

	oldUmask := 0
	if network == "unix" {
		oldUmask = syscall.Umask(0o177)
	}
	lis, err := (&net.ListenConfig{}).Listen(ctx, network, address)
	if network == "unix" {
		syscall.Umask(oldUmask)
	}
	if err != nil {
		logger.Error("failed to listen", slog.String("network", network), slog.String("address", address), slog.Any("error", err))
		return
	}

	mux := http.NewServeMux()
	path, handler := workspacev1connect.NewWorkspaceExecutorServiceHandler(executorsvc.New(executorsvc.Options{
		DefaultWorkDir:    executorsvc.DefaultWorkDir,
		DataMount:         executorsvc.DefaultWorkDir,
		AllowHostAbsolute: true,
	}))
	mux.Handle(path, handler)

	server := &http.Server{
		Handler:           h2c.NewHandler(mux, &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Info("workspace executor listening", slog.String("network", network), slog.String("address", address))
	if err := server.Serve(lis); err != nil && !errorsIsServerClosed(err) {
		logger.Error("workspace executor server failed", slog.Any("error", err))
	}
}

func reapZombies() {
	var status syscall.WaitStatus
	for {
		if _, err := syscall.Wait4(-1, &status, 0, nil); err != nil {
			time.Sleep(time.Second)
		}
	}
}

func errorsIsServerClosed(err error) bool {
	return err == nil || errors.Is(err, http.ErrServerClosed)
}
