package main

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "serve [memoh-server args]",
		Short:              "Start the Memoh server daemon",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServerDaemon(cmd.Context(), args)
		},
	}
	return cmd
}

func runServerDaemon(ctx context.Context, args []string) error {
	if len(args) > 0 {
		return errors.New("memoh serve does not accept passthrough server args")
	}
	command := exec.CommandContext(ctx, "memoh-server", "serve")
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = os.Environ()
	return command.Run()
}
