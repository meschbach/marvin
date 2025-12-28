package query

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
)

func ListMCPTools(ctx context.Context, cfg *config.File, detailed bool) {
	tools, err := NewToolSet(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading tools: %v\n", err)
		return
	}
	defer tools.Shutdown(ctx)

	for _, instruction := range tools.instructions {
		fmt.Printf("Instruction: %s\n=== End instruction ===\n", instruction.Content)
	}
	if len(tools.instructions) == 0 {
		fmt.Println("No instructions found")
	}
	fmt.Println()

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
}
