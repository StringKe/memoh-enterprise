package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/memohai/memoh/internal/logger"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		logger.Error("connector failed", slog.Any("error", err))
		os.Exit(1)
	}
}
