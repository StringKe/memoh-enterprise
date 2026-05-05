package main

import (
	"github.com/spf13/cobra"

	"github.com/memohai/memoh/internal/cli"
)

type cliContext struct {
	state  cli.State
	server string
}

func newRootCommand() *cobra.Command {
	ctx := &cliContext{}

	rootCmd := &cobra.Command{
		Use:   "memoh",
		Short: "Memoh terminal operator CLI",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			state, err := cli.LoadState()
			if err != nil {
				return err
			}
			ctx.state = state
			if ctx.server != "" {
				ctx.state.ServerURL = cli.NormalizeServerURL(ctx.server)
			}
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&ctx.server, "server", "", "Memoh server URL")

	rootCmd.AddCommand(newMigrateCommand())
	rootCmd.AddCommand(newInstallCommand())
	rootCmd.AddCommand(newLoginCommand(ctx))
	rootCmd.AddCommand(newChatCommand(ctx))
	rootCmd.AddCommand(newBotsCommand(ctx))
	rootCmd.AddCommand(newComposeCommands()...)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runVersion()
		},
	})

	return rootCmd
}
