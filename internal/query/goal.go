package query

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"
)

const mcpParameterTypeObject = "object"
const mcpParameterTypeString = "string"

func PerformGoalWithConfig(cfg *config.File, goal string) {
	ctx, done := context.WithCancel(context.Background())
	defer done()

	realToolSet, err := loadToolsFromConfig(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading MCP servers: %v\n", err)
		return
	}
	defer shutdownToolSet(ctx, realToolSet, "real")

	reasoningToolset, err := createReasoningToolSet(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Ollama client: %v\n", err)
		return
	}
	defer shutdownToolSet(ctx, reasoningToolset, "reasoning")

	fmt.Printf("Goal: %s\n", goal)

	client, err := api.ClientFromEnvironment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Ollama client: %v\n", err)
		return
	}

	availableTools := formatAvailableTools(realToolSet.Defs)
	messages := buildGoalMessages(availableTools, goal)

	engine := conversation.NewEngine(client, cfg, &conversation.NullLogger{}, reasoningToolset, messages)

	model := "ministral-3:3b"
	if cfg != nil && cfg.Model != "" {
		model = cfg.Model
	}

	updater := NewCLIStreamingUpdater(false, false, false, "plain")

	if err := engine.RunConversation(ctx, model, updater); err != nil {
		fmt.Fprintf(os.Stderr, "Error running AI: %v\n", err)
	}
}

func shutdownToolSet(ctx context.Context, ts *conversation.ToolSet, name string) {
	if err := ts.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error shutting down %s tool set: %v\n", name, err)
	}
}

func createReasoningToolSet(ctx context.Context, cfg *config.File) (*conversation.ToolSet, error) {
	ts, err := loadToolsFromConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := ts.RegisterTool(ctx, &questionForUser{}); err != nil {
		fmt.Fprintf(os.Stderr, "Error registering question for user tool: %v\n", err)
		return nil, err
	}

	return ts, nil
}

func formatAvailableTools(tools api.Tools) string {
	availableTools := "These are tools available for the instructed AI:\n"
	for _, tool := range tools {
		availableTools += fmt.Sprintf("\t%s: %s\n", tool.Function.Name, tool.Function.Description)
	}
	return availableTools
}

func buildGoalMessages(availableTools string, goal string) []api.Message {
	return []api.Message{
		{
			Role:    conversation.RoleSystem,
			Content: "You are an expert system in reasoning through problems.  You are building an instruction list for another AI and may only call steps starting with 'reasoning' .  Enumerate each step to be achieved via reasoning_step tool.  When you need further clarification or more information request this via reasoning_clairifying_question tool.  If instructions are clear then do not ask any clairifying questions.",
		},
		{Role: conversation.RoleSystem, Content: availableTools},
		{Role: conversation.RoleUser, Content: goal},
	}
}

type questionForUser struct {
}

func (q questionForUser) Invoke(ctx context.Context, call api.ToolCall) (out []api.Message, problem error) {
	args := call.Function.Arguments
	prompt, hasPrompt := args.Get("prompt")
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
			Role:       conversation.RoleToolResult,
			Content:    "",
			ToolName:   call.Function.Name,
			ToolCallID: call.ID,
		},
		{
			Role:    conversation.RoleUser,
			Content: trimmedInput,
		},
	}, nil
}

func (q questionForUser) DefineAPI(ctx context.Context) (definition *conversation.ToolDefinition, problem error) {
	props := api.NewToolPropertiesMap()
	props.Set("prompt", api.ToolProperty{
		Type:        []string{mcpParameterTypeString},
		Description: "The prompt to ask the user",
	})
	definitions := conversation.NewToolDefinition()
	definitions.Tool = api.Tools{
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "reasoning_clairifying_question",
				Description: "Request clarification from the user or to better understand what the instructions are",
				Parameters: api.ToolFunctionParameters{
					Type:       mcpParameterTypeObject,
					Required:   []string{"prompt"},
					Properties: props,
				},
			},
		},
	}
	definitions.AppendInstruction("Use the tool reasoning_clairifying_question to ask the user for additional details when you are unsure, need more information, or are otherwise not certain.")
	return definitions, nil
}
