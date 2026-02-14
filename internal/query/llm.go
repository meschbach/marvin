package query

import (
	"context"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/gemini"
	"github.com/meschbach/marvin/internal/openrouter"
	"github.com/ollama/ollama/api"
)

type OllamaLLM struct {
	client *api.Client
}

func NewOllamaLLM(client *api.Client) *OllamaLLM {
	return &OllamaLLM{client: client}
}

func (o *OllamaLLM) Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	return o.client.Chat(ctx, req, fn)
}

func NewLLM(cfg *config.File) (conversation.LLM, error) {
	switch cfg.Provider() {
	case config.ProviderGemini:
		return newGeminiLLM(cfg)
	case config.ProviderOpenRouter:
		return newOpenRouterLLM(cfg)
	case config.ProviderOllama:
		fallthrough
	default:
		return newOllamaLLMFromEnv()
	}
}

func newGeminiLLM(cfg *config.File) (conversation.LLM, error) {
	apiKey, err := cfg.ResolveGeminiAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Gemini API key: %w", err)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("gemini API key is required. Set it via config, file, or GEMINI_API_KEY env var")
	}

	return gemini.NewLLM(apiKey, cfg.LanguageModel())
}

func newOpenRouterLLM(cfg *config.File) (conversation.LLM, error) {
	apiKey, err := cfg.ResolveOpenRouterAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve OpenRouter API key: %w", err)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("openrouter API key is required. Set it via config, file (--openrouter-key-file), or OPENROUTER_API_KEY env var")
	}

	baseURL := ""
	if cfg.OpenRouter != nil && cfg.OpenRouter.BaseURL != "" {
		baseURL = cfg.OpenRouter.BaseURL
	}

	return openrouter.NewLLM(apiKey, baseURL, cfg.LanguageModel()), nil
}

func newOllamaLLMFromEnv() (conversation.LLM, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create Ollama client: %w", err)
	}
	return NewOllamaLLM(client), nil
}
