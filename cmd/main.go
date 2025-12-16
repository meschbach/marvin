package main

import (
	"github.com/meschbach/marvin/internal/config"
	"github.com/spf13/cobra"
)

type globalOptions struct {
	config *config.CommandLineOptions
}

func main() {
	globalOpts := &globalOptions{
		config: config.NewCommandLineOptions(),
	}

	mcpList := mcpListCommand(globalOpts)

	mcp := &cobra.Command{
		Use: "mcp",
	}
	mcp.AddCommand(mcpList)

	queryCmd := queryCommand(globalOpts)
	goalCmd := goalCommand(globalOpts)

	root := &cobra.Command{
		Use:   "marvin",
		Short: "An AI workbench experiment backed by ollama",
	}
	globalOpts.config.PersistentFlags(root)

	root.AddCommand(mcp)
	root.AddCommand(queryCmd)
	root.AddCommand(goalCmd)
	root.AddCommand(ragCommand(globalOpts))

	if err := root.Execute(); err != nil {
		panic(err)
	}
}
