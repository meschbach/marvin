package query

import (
	"context"
	"fmt"
	"os"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
)

// PerformWithConfig executes the search using the optional parsed configuration.
func PerformWithConfig(cfg *config.File, actualQuery string, showThinking bool) {
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
	messages := []api.Message{
		systemMessage,
		{Role: "user", Content: actualQuery},
	}
	availableTools := toolset.APITools()

	// ctx already defined above
	conversation := &ollamaConversation{
		client:       client,
		messages:     messages,
		tools:        toolset,
		showThinking: showThinking,
	}
	model := cfg.LanguageModel()

	if err := conversation.runAIToConclusion(ctx, model, availableTools); err != nil {
		return
	}
}
