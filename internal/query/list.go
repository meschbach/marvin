package query

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
)

func NewListMCPTools() *cobra.Command {
	cmd := &cobra.Command{
		Use: "list",
		Run: func(cmd *cobra.Command, args []string) {
			var cfg *Config
			configPath, _ := cmd.Flags().GetString("config")
			if configPath != "" {
				parsed, err := LoadConfig(configPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error loading config %q: %v\n", configPath, err)
					return
				}
				cfg = parsed
			}

			detailed, err := cmd.Flags().GetBool("detailed")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing detailed flag %v\n", err)
				return
			}

			ctx, done := context.WithCancel(cmd.Context())
			defer done()
			tools, err := NewToolSet(ctx, cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading tools: %v\n", err)
				return
			}

			for _, tool := range tools.defs {
				fmt.Printf("%s: %s\n", tool.Function.Name, tool.Function.Description)
				dumpLayer := func(prefix string, p api.ToolFunctionParameters) {
					prefix = prefix + "\t"
					fmt.Printf("%s%s\n", prefix, p.Type)
					for name, prop := range p.Properties {
						var optionalRequiredText string
						if slices.Contains(p.Required, name) {
							optionalRequiredText = "(required)"
						} else {
							optionalRequiredText = ""
						}
						fmt.Printf("%s%s: %s %s\n", prefix, name, prop.Description, optionalRequiredText)
					}
				}
				if detailed {
					dumpLayer("\t", tool.Function.Parameters)
				}
			}
		},
	}
	cmd.Flags().StringP("config", "c", "", "Path to configuration file (HCL)")
	cmd.Flags().Bool("detailed", false, "Provides detailed output for the tool")
	return cmd
}
