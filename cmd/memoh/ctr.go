package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func newContainerdCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "ctr [ctr args]",
		Aliases:            []string{"containerd"},
		Short:              "Manage the nested containerd inside the server container",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		Long: `Run ctr inside the Docker Compose server service so you can inspect and
manage the nested containerd that Memoh uses for workspace containers.

By default this command injects the containerd namespace from config.toml.
Pass --no-namespace or provide your own ctr -n/--namespace flag to override it.`,
		Example: `  memoh ctr images ls
  memoh ctr containers ls
  memoh ctr --namespace default tasks ls
  memoh ctr --server-service server -- snapshots ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || isHelpArg(args[0]) {
				return cmd.Help()
			}

			opts, ctrArgs, err := parseContainerdOptions(args)
			if err != nil {
				return err
			}
			if len(ctrArgs) == 0 {
				return errors.New("missing ctr arguments")
			}

			return runServerContainerd(cmd.Context(), opts, ctrArgs)
		},
	}

	return cmd
}
