package query

import (
	"context"
	"fmt"
	"os"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/llm"
)

type ChatOptions struct {
	Verbose bool
	//ShowTools will print out tool utilization and integration
	ShowTools bool
	//DumpTooling will print out tooling available
	DumpTooling bool
	//ShowThinking will print out thinking process
	ShowThinking bool
	//ShowDone will print out when the LLM issues a "Done" command
	ShowDone bool
	//ThinkingFormat determines how thinking content is formatted
	ThinkingFormat string
}

// PerformWithConfig executes the search using the optional parsed configuration.
func PerformWithConfig(ctx context.Context, cfg *config.File, actualQuery string, opts *ChatOptions) {
	// Apply configuration defaults if CLI flags not set
	if cfg != nil {
		if !opts.ShowThinking && cfg.ShowThinking() {
			opts.ShowThinking = true
		}
		if !opts.ShowTools && cfg.ShowTools() {
			opts.ShowTools = true
		}
		if !opts.ShowDone && cfg.ShowDone() {
			opts.ShowDone = true
		}
		if !opts.Verbose && cfg.Verbose() {
			opts.Verbose = true
		}
		if opts.ThinkingFormat == "" {
			opts.ThinkingFormat = cfg.ThinkingFormat()
		}
	}

	if opts.Verbose {
		fmt.Printf("user search:\t%s\n", actualQuery)
	}

	// Create LLM client based on configuration (supports Ollama and OpenRouter)
	llmClient, err := NewLLM(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating LLM client: %v\n", err)
		return
	}

	// Build tools from configuration (if provided)
	toolset, warnings := loadToolsFromConfig(ctx, cfg)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", w)
	}
	defer func() {
		fmt.Println("Shutting down tools")
		if err := toolset.Shutdown(ctx); err != nil {
			fmt.Printf("Error shutting down tools: %v\n", err)
		}
	}()
	for _, rag := range cfg.Documents {
		tool := &ChromemTool{config: rag, showInvocations: false}
		if err := toolset.RegisterTool(ctx, tool); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			continue
		}
	}

	systemMessageContent := "You are a helpful assistant."
	if cfg != nil && cfg.SystemPrompt != nil {
		if len(cfg.SystemPrompt.FromString) > 0 {
			systemMessageContent = cfg.SystemPrompt.FromString
		}
		if len(cfg.SystemPrompt.FromFile) > 0 {
			contents, err := os.ReadFile(cfg.SystemPrompt.FromFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading system prompt file %q: %v\n", cfg.SystemPrompt.FromFile, err)
				return
			}
			systemMessageContent = string(contents)
		}
	}

	systemMessage := llm.Message{
		Role: conversation.RoleSystem, Content: systemMessageContent,
	}

	// Maintain the rolling chat messages to support tool-call loops
	availableTools := toolset.APITools()
	for _, tool := range availableTools {
		if opts.DumpTooling {
			fmt.Printf("\t%s: %s\n", tool.Function.Name, tool.Function.Description)
		}
		toolset.Instructions = append(toolset.Instructions, llm.Message{Role: conversation.RoleAssistant, Content: fmt.Sprintf("Function %s %s", tool.Function.Name, tool.Function.Description)})
	}
	messages := append(toolset.Instructions,
		systemMessage,
		llm.Message{Role: conversation.RoleUser, Content: actualQuery},
	)

	if opts.ShowTools {
		fmt.Printf("Initial Messages (%d):\n", len(messages))
		for _, m := range messages {
			fmt.Printf("\t%s: %s\n", m.Role, m.Content)
		}
	}

	// Create the new conversation engine
	updater := NewCLIStreamingUpdater(opts.ShowThinking, opts.ShowTools, opts.ShowDone, opts.ThinkingFormat)

	var logger conversation.Logger
	if opts.Verbose {
		logger = &conversation.VerboseLogger{}
	} else {
		logger = &conversation.NullLogger{}
	}

	engine := conversation.NewEngine(
		llmClient,
		cfg,
		logger,
		toolset,
		messages,
	)

	model := cfg.LanguageModel()
	if opts.Verbose {
		fmt.Printf("config\t> model: %s\n", model)
	}

	if err := engine.RunConversation(ctx, model, updater); err != nil {
		return
	}
}
