package main

import (
	"github.com/meschbach/marvin/internal/query"
	"github.com/spf13/cobra"
)

func main() {
	mcpList := &cobra.Command{
		Use: "list",
		Run: func(cmd *cobra.Command, args []string) {

		},
	}

	mcp := &cobra.Command{
		Use: "mcp",
	}
	mcp.AddCommand(mcpList)

	queryCmd := query.New()
	goalCmd := query.NewGoalCommand()

	root := &cobra.Command{
		Use:   "marvin",
		Short: "An AI workbench experiment backed by ollama",
	}
	root.AddCommand(mcp)
	root.AddCommand(queryCmd)
	root.AddCommand(goalCmd)

	if err := root.Execute(); err != nil {
		panic(err)
	}
}
