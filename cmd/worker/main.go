package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/memohai/memoh/internal/logger"
)

func main() {
	if err := run(context.Background()); err != nil {
		logger.Error("worker failed", slog.Any("error", err))
		os.Exit(1)
	}
}
