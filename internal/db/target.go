package db

import "github.com/memohai/memoh/internal/config"

const (
	DriverPostgres = "postgres"
)

type MigrationTarget struct {
	Driver string
	DSN    string
}

func DriverFromConfig(cfg config.Config) string {
	_ = cfg
	return DriverPostgres
}

func MigrationTargetFromConfig(cfg config.Config) (MigrationTarget, error) {
	return MigrationTarget{Driver: DriverPostgres, DSN: DSN(cfg.Postgres)}, nil
}
