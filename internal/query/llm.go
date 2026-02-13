package query

import (
	"context"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/openrouter"
	"github.com/ollama/ollama/api"
)

// OllamaLLM wraps the Ollama client to implement the LLM interface
type OllamaLLM struct {
	client *api.Client
}

// NewOllamaLLM creates a new LLM implementation using the Ollama client
func NewOllamaLLM(client *api.Client) *OllamaLLM {
	return &OllamaLLM{client: client}
}

// Chat implements the LLM interface by delegating to the Ollama client
func (o *OllamaLLM) Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	return o.client.Chat(ctx, req, fn)
}

// NewLLM creates an LLM instance based on the configuration.
// Returns an error if OpenRouter is selected but no API key is available.
func NewLLM(cfg *config.File) (conversation.LLM, error) {
	switch cfg.Provider() {
	case config.ProviderOpenRouter:
		apiKey, err := cfg.ResolveOpenRouterAPIKey()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve OpenRouter API key: %w", err)
		}
		if apiKey == "" {
			return nil, fmt.Errorf("OpenRouter API key is required. Set it via config, file (--openrouter-key-file), or OPENROUTER_API_KEY env var")
		}

		baseURL := ""
		if cfg.OpenRouter != nil && cfg.OpenRouter.BaseURL != "" {
			baseURL = cfg.OpenRouter.BaseURL
		}

		return openrouter.NewLLM(apiKey, baseURL, cfg.LanguageModel()), nil

	case config.ProviderOllama:
		fallthrough
	default:
		client, err := api.ClientFromEnvironment()
		if err != nil {
			return nil, fmt.Errorf("failed to create Ollama client: %w", err)
		}
		return NewOllamaLLM(client), nil
	}
}
