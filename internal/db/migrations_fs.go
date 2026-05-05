package db

import (
	"io/fs"

	"github.com/memohai/memoh/internal/config"
)

func MigrationsFSForConfig(cfg config.Config, embedded fs.FS) (fs.FS, error) {
	_ = cfg
	return fs.Sub(embedded, "postgres/migrations")
}
