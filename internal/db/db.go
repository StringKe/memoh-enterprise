package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/config"
)

func Open(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	return OpenPostgres(ctx, cfg.Postgres)
}

func OpenPostgres(ctx context.Context, cfg config.PostgresConfig) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, DSN(cfg))
}
