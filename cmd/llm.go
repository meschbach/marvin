package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/meschbach/marvin/internal/query"
	"github.com/spf13/cobra"
)

func queryCommand(global *globalOptions) *cobra.Command {
	type queryOptions struct {
		showThinking bool
	}
	queryOpts := &queryOptions{}

	cmd := &cobra.Command{
		Use:   "query <query...>",
		Short: "Send a free-form query to Ollama and print the response",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			actualQuery := strings.Join(args, " ")
			if actualQuery == "" {
				fmt.Fprintln(os.Stderr, "No query provided")
				_ = cmd.Help()
				return
			}

			config, err := global.config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				return
			}
			query.PerformWithConfig(config, actualQuery, queryOpts.showThinking)
		},
	}
	pflags := cmd.PersistentFlags()
	pflags.BoolVarP(&queryOpts.showThinking, "show-thinking", "t", false, "Show the models thinking")
	return cmd
}

func goalCommand(global *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "goal <goal...>",
		Short: "Declare a high-level goal for the current session",
		Long:  "Declare a high-level goal for the current session. This command currently echoes the goal text.",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			goal := strings.Join(args, " ")
			fmt.Println(goal)
			config, err := global.config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				return
			}

			query.PerformGoalWithConfig(config, goal)
		},
	}
	return cmd
}
