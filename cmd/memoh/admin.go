package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/db"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	"github.com/memohai/memoh/internal/integrations"
	"github.com/memohai/memoh/internal/logger"
)

func newAdminCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Run local break-glass administration commands",
	}
	cmd.AddCommand(newAdminResetPasswordCommand())
	cmd.AddCommand(newAdminDisableIntegrationTokensCommand())
	return cmd
}

func newAdminResetPasswordCommand() *cobra.Command {
	var identity string
	var password string

	cmd := &cobra.Command{
		Use:   "reset-password",
		Short: "Reset a local user password from the configured database",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAdminResetPassword(cmd.Context(), identity, password)
		},
	}
	cmd.Flags().StringVar(&identity, "user", "", "Username or email to reset")
	cmd.Flags().StringVar(&password, "password", "", "New password")
	return cmd
}

func newAdminDisableIntegrationTokensCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "disable-integration-tokens",
		Short: "Disable all enterprise integration API tokens",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAdminDisableIntegrationTokens(cmd.Context())
		},
	}
}

func runAdminResetPassword(ctx context.Context, identity string, password string) error {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return errors.New("--user is required")
	}
	if strings.TrimSpace(password) == "" {
		return errors.New("--password is required")
	}
	store, closeStore, err := openAdminStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	account, err := store.GetByIdentity(ctx, identity)
	if err != nil {
		return fmt.Errorf("find user %q: %w", identity, err)
	}
	service := accounts.NewService(logger.L, store)
	if err := service.ResetPassword(ctx, account.ID, password); err != nil {
		return fmt.Errorf("reset password for %q: %w", identity, err)
	}
	fmt.Printf("password reset for %s\n", identity)
	return nil
}

func runAdminDisableIntegrationTokens(ctx context.Context) error {
	store, closeStore, err := openAdminStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	service := integrations.NewService(logger.L, postgresstore.NewQueries(store.SQLC()))
	if err := service.DisableAllAPITokens(ctx); err != nil {
		return fmt.Errorf("disable integration tokens: %w", err)
	}
	fmt.Println("all integration API tokens disabled")
	return nil
}

func openAdminStore(ctx context.Context) (*postgresstore.Store, func(), error) {
	cfg, err := provideConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	logger.Init(cfg.Log.Level, cfg.Log.Format)
	pool, err := db.Open(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	store, err := postgresstore.New(pool)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("open store: %w", err)
	}
	return store, pool.Close, nil
}
