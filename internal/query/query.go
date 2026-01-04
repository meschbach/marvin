package query

import (
	"context"
	"fmt"
	"os"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
)

type ChatOptions struct {
	//ShowTools will print out tool utilization and integration
	ShowTools bool
	//DumpTooling will print out the tooling available
	DumpTooling bool
	//ShowThinking will print out the thinking process
	ShowThinking bool
	//ShowDone will print out when the LLM issues a "Done" command
	ShowDone bool
}

// PerformWithConfig executes the search using the optional parsed configuration.
func PerformWithConfig(cfg *config.File, actualQuery string, opts *ChatOptions) {
	fmt.Printf("user search:\t%s\n", actualQuery)

	// search Ollama for a response
	client, err := api.ClientFromEnvironment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Ollama client: %v\n", err)
		return
	}

	// Build tools from configuration (if provided)
	ctx := context.Background()
	toolset, tsErr := NewToolSet(ctx, cfg)
	if tsErr != nil {
		fmt.Fprintf(os.Stderr, "Error initializing tools: %v\n", tsErr)
		return
	}
	defer func() {
		fmt.Println("Shutting down tools")
		if err := toolset.Shutdown(ctx); err != nil {
			fmt.Printf("Error shutting down tools: %v\n", err)
		}
	}()
	for _, rag := range cfg.Documents {
		tool := &chromemTool{config: rag, showInvocations: false}
		if err := toolset.registerTool(ctx, tool); err != nil {
			fmt.Fprintf(os.Stderr, "Error registering RAG tool: %v\n", err)
			return
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

	systemMessage := api.Message{
		Role: "system", Content: systemMessageContent,
	}

	// Maintain the rolling chat messages to support tool-call loops
	messages := append(toolset.instructions,
		systemMessage,
		api.Message{Role: "user", Content: actualQuery})
	availableTools := toolset.APITools()
	if opts.DumpTooling {
		fmt.Println("Available tools:")
		for _, tool := range availableTools {
			fmt.Printf("\t%s: %s\n", tool.Function.Name, tool.Function.Description)
		}
	}

	// ctx already defined above
	conversation := &ollamaConversation{
		client:       client,
		messages:     messages,
		tools:        toolset,
		showThinking: opts.ShowThinking,
		showTools:    opts.ShowTools,
		showDone:     opts.ShowDone,
	}
	model := cfg.LanguageModel()
	fmt.Printf("config\t> model: %s\n", model)

	if err := conversation.runAIToConclusion(ctx, model, availableTools); err != nil {
		return
	}
}
