package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/meschbach/marvin/internal/query"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

func mcpListCommand(global *globalOptions) *cobra.Command {
	type options struct {
		detailed bool
	}
	opts := &options{}

	cmd := &cobra.Command{
		Use: "list",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, done := signal.NotifyContext(cmd.Context(), unix.SIGSTOP, unix.SIGINT, unix.SIGTERM)
			defer done()

			cfg, err := global.config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				return
			}
			query.ListMCPTools(ctx, cfg, opts.detailed)
		},
	}
	pflags := cmd.PersistentFlags()
	pflags.BoolVarP(&opts.detailed, "detailed", "d", false, "Provides detailed output for the tool")
	return cmd
}
