package query

import (
	"bufio"
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
const roleToolResult = "tool_result"

const mcpParameterTypeObject = "object"
const mcpParameterTypeString = "string"

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

			var cfg *Config
			configPath, _ := cmd.Flags().GetString("config")
			if configPath != "" {
				parsed, err := LoadConfig(configPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error loading config %q: %v\n", configPath, err)
					return nil
				}
				cfg = parsed
			}
			realToolSet, err := NewToolSet(ctx, cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading MCP servers: %v\n", err)
				return nil
			}

			reasoningToolset, err := NewToolSet(cmd.Context(), nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating Ollama client: %v\n", err)
				return nil
			}
			//if err := reasoningToolset.registerTool(ctx, &reasoningStep{}); err != nil {
			//	fmt.Fprintf(os.Stderr, "Error registering reasoning step tool: %v\n", err)
			//	return err
			//}
			if err := reasoningToolset.registerTool(ctx, &questionForUser{}); err != nil {
				fmt.Fprintf(os.Stderr, "Error registering question for user tool: %v\n", err)
				return err
			}

			goal := strings.Join(args, " ")
			fmt.Printf("Goal: %s\n", goal)

			// Query Ollama for a response
			client, err := api.ClientFromEnvironment()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating Ollama client: %v\n", err)
				return nil
			}

			//generate a message of available MCP tools
			availableTools := "These are tools available for the instructed AI:\n"
			for _, tool := range realToolSet.defs {
				availableTools += fmt.Sprintf("\t%s: %s\n", tool.Function.Name, tool.Function.Description)
			}

			// Query the AI model for the steps required to complete the goal
			stepsConversation := ollamaConversation{
				client: client,
				messages: []api.Message{
					{
						Role:    roleSystem,
						Content: "You are an expert system in reasoning through problems.  You are building an instruction list for another AI and may only call steps starting with 'reasoning' .  Enumerate each step to be achieved via the reasoning_step tool.  When you need further clarification or more information request this via reasoning_clairifying_question tool.  If instructions are clear then do not ask any clairifying questions.",
					},
					{Role: roleSystem, Content: availableTools},
					{Role: roleUser, Content: goal},
				},
				tools: reasoningToolset,
			}

			model := "ministral-3:3b"
			if cfg != nil && cfg.Model != "" {
				model = cfg.Model
			}

			if err := stepsConversation.runAIToConclusion(ctx, model, reasoningToolset.defs); err != nil {
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

type questionForUser struct {
}

func (q questionForUser) invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	args := call.Function.Arguments
	prompt, hasPrompt := args["prompt"]
	if !hasPrompt {
		return nil, fmt.Errorf("missing required argument 'prompt'")
	}

	fmt.Printf("ai> %s\n", prompt)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	trimmedInput := strings.TrimSpace(input)
	return []api.Message{
		{
			Role:       roleToolResult,
			Content:    "",
			ToolName:   call.Function.Name,
			ToolCallID: call.ID,
		},
		{
			Role:    roleUser,
			Content: trimmedInput,
		},
	}, nil
}

func (q questionForUser) defineAPI(ctx context.Context) (tool api.Tools, problem error) {
	output := api.Tools{
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "reasoning_clairifying_question",
				Description: "Request clarification from the user or to better understand what the instructions are",
				Parameters: api.ToolFunctionParameters{
					Type:     mcpParameterTypeObject,
					Required: []string{"prompt"},
					Properties: map[string]api.ToolProperty{
						"prompt": {
							Type:        []string{mcpParameterTypeString},
							Description: "The prompt to ask the user",
						},
					},
				},
			},
		},
	}
	return output, nil
}
