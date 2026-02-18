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
	tools, err := loadToolsFromConfig(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading tools: %v\n", err)
		return
	}
	defer func() {
		if err := tools.Shutdown(ctx); err != nil {
			fmt.Printf("Error shutting down tools: %v\n", err)
		}
	}()

	printInstructions(tools.Instructions)
	printTools(tools.Defs, detailed)
}

func printInstructions(instructions []api.Message) {
	for _, instruction := range instructions {
		fmt.Printf("Instruction: %s\n=== End instruction ===\n", instruction.Content)
	}
	if len(instructions) == 0 {
		fmt.Println("No instructions found")
	}
	fmt.Println()
}

func printTools(tools api.Tools, detailed bool) {
	for _, tool := range tools {
		fmt.Printf("%s: %s\n", tool.Function.Name, tool.Function.Description)
		if detailed {
			dumpParameters("\t", tool.Function.Parameters)
		}
	}
}

func dumpParameters(prefix string, p api.ToolFunctionParameters) {
	prefix = prefix + "\t"
	fmt.Printf("%s%s\n", prefix, p.Type)
	for name, prop := range p.Properties.All() {
		var optionalRequiredText string
		if slices.Contains(p.Required, name) {
			optionalRequiredText = "(required)"
		} else {
			optionalRequiredText = ""
		}
		fmt.Printf("%s%s: %s %s\n", prefix, name, prop.Description, optionalRequiredText)
	}
}
