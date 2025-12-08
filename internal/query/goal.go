package query

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
)

const roleSystem = "system"
const roleUser = "user"
const roleAssistant = "assistant"

// NewGoalCommand creates the `goal` command. This command records or echoes a
// high-level goal provided by the user. For now, it simply prints the goal to
// stdout so it can be piped or observed by other tooling.
func NewGoalCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "goal <goal...>",
		Short: "Declare a high-level goal for the current session",
		Long:  "Declare a high-level goal for the current session. This command currently echoes the goal text.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, done := context.WithCancel(cmd.Context())
			defer done()

			//var cfg *Config
			//configPath, _ := cmd.Flags().GetString("config")
			//if configPath != "" {
			//	parsed, err := LoadConfig(configPath)
			//	if err != nil {
			//		fmt.Fprintf(os.Stderr, "Error loading config %q: %v\n", configPath, err)
			//		return nil
			//	}
			//	cfg = parsed
			//}
			reasoningToolset, err := NewToolSet(cmd.Context(), nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating Ollama client: %v\n", err)
				return nil
			}
			reasoningToolset.registerTool(ctx, &reasoningStep{})

			goal := strings.Join(args, " ")
			fmt.Printf("Goal: %s\n", goal)

			// Query Ollama for a response
			client, err := api.ClientFromEnvironment()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating Ollama client: %v\n", err)
				return nil
			}

			// Query the AI model for the steps required to complete the goal
			stepsConversation := ollamaConversation{
				client: client,
				messages: []api.Message{
					{Role: roleSystem, Content: "You are an expert system describing how to achieve a user goal.  Enumerate each step to be achieved via the reasoning_step tool"},
					{Role: roleUser, Content: goal},
				},
				tools: reasoningToolset,
			}
			if err := stepsConversation.runAIToConclusion(ctx, reasoningToolset.defs); err != nil {
				fmt.Fprintf(os.Stderr, "Error running AI: %v\n", err)
				return nil
			}
			return nil
		},
	}
	cmd.Flags().StringP("config", "c", "", "Path to configuration file (HCL)")
	return cmd
}

type reasoningStep struct {
	steps []string
}

func (r reasoningStep) invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	fmt.Printf("invoked reasoning step with %s\n", call.Function.Arguments.String())
	return []api.Message{
		{
			Role:       "tool_result",
			Content:    call.Function.Arguments.String(),
			ToolName:   call.Function.Name,
			ToolCallID: call.ID,
		},
	}, nil
}

func (r reasoningStep) defineAPI(ctx context.Context) (tool api.Tools, problem error) {
	return api.Tools{
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "reasoning_step",
				Description: "Defines a small and finite step to approach the problem",
				Parameters: api.ToolFunctionParameters{
					Type:     "object",
					Required: []string{"step"},
					Properties: map[string]api.ToolProperty{
						"step": {
							Type:        []string{"string"},
							Description: "Defines the step to be taken",
						},
					},
				},
			},
		},
	}, nil
}
