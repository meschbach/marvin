package query

import (
	"context"
	"fmt"
	"os"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
)

var truthful = true
var truePtr = &truthful

// PerformWithConfig executes the query using the optional parsed configuration.
func PerformWithConfig(cfg *config.File, actualQuery string) {
	fmt.Printf("user query:\t%s\n", actualQuery)

	// query Ollama for a response
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
	//fmt.Println("Available tools:")
	//for _, tool := range availableTools {
	//	fmt.Printf("%s: %s\n", tool.Function.Name, tool.Function.Description)
	//}

	// ctx already defined above
	conversation := &ollamaConversation{
		client:   client,
		messages: messages,
		tools:    toolset,
	}
	model := "ministral-3:3b"
	if cfg != nil && cfg.Model != "" {
		model = cfg.Model
	}

	if err := conversation.runAIToConclusion(ctx, model, availableTools); err != nil {
		return
	}
}
