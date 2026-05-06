package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/memohai/memoh/internal/version"
)

func newSupportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "support",
		Short: "Print support diagnostics",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runSupportDiagnostics()
		},
	}
	return cmd
}

func runSupportDiagnostics() error {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.toml"
	}
	fmt.Printf("memoh: %s\n", version.GetInfo())
	fmt.Printf("goos: %s\n", runtime.GOOS)
	fmt.Printf("goarch: %s\n", runtime.GOARCH)
	fmt.Printf("config_path: %s\n", configPath)
	fmt.Printf("server_bin: %s\n", envDefault("MEMOH_SERVER_BIN", "memoh-server"))
	return nil
}

func envDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
