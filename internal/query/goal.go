package query

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/llm"
	llmfactory "github.com/meschbach/marvin/internal/llm/factory"
)

const mcpParameterTypeObject = "object"
const mcpParameterTypeString = "string"

func PerformGoalWithConfig(cfg *config.File, goal string) {
	ctx, done := context.WithCancel(context.Background())
	defer done()

	realToolSet, warnings := loadToolsFromConfig(context.Background(), cfg)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", w)
	}
	defer shutdownToolSet(ctx, realToolSet, "real")

	reasoningToolset, err := createReasoningToolSet(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Ollama client: %v\n", err)
		return
	}
	defer shutdownToolSet(ctx, reasoningToolset, "reasoning")

	fmt.Printf("Goal: %s\n", goal)

	llmClient, err := llmfactory.NewFromConfig(ctx, cfg, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating LLM client: %v\n", err)
		return
	}

	availableTools := formatAvailableTools(realToolSet.Defs)
	messages := buildGoalMessages(availableTools, goal)

	engine := conversation.NewEngine(llmClient, cfg, &conversation.NullLogger{}, reasoningToolset, messages)

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
	ts, warnings := loadToolsFromConfig(ctx, cfg)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", w)
	}

	if err := ts.RegisterTool(ctx, &questionForUser{}); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		// Continue anyway - the questionForUser tool is important but not critical
	}

	return ts, nil
}

func formatAvailableTools(tools []llm.ToolDefinition) string {
	availableTools := "These are tools available for the instructed AI:\n"
	for _, tool := range tools {
		availableTools += fmt.Sprintf("\t%s: %s\n", tool.Function.Name, tool.Function.Description)
	}
	return availableTools
}

func buildGoalMessages(availableTools string, goal string) []llm.Message {
	return []llm.Message{
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

func (q questionForUser) Invoke(ctx context.Context, call llm.ToolCall) (out []llm.Message, problem error) {
	args, ok := call.Function.Arguments.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing required argument 'prompt'")
	}
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
	return []llm.Message{
		{
			Role:       conversation.RoleToolResult,
			Content:    "",
			ToolCallID: call.ID,
			ToolName:   call.Function.Name,
		},
		{
			Role:    conversation.RoleUser,
			Content: trimmedInput,
		},
	}, nil
}

func (q questionForUser) DefineAPI(ctx context.Context) (definition *conversation.ToolDefinition, problem error) {
	props := map[string]llm.ToolProperty{
		"prompt": {
			Type:        []string{mcpParameterTypeString},
			Description: "The prompt to ask the user",
		},
	}
	definitions := conversation.NewToolDefinition()
	definitions.Tool = []llm.ToolDefinition{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "reasoning_clairifying_question",
				Description: "Request clarification from the user or to better understand what the instructions are",
				Parameters: &llm.ToolFunctionParameters{
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
