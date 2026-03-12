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

func (o *OllamaLLM) Chat(ctx context.Context, req *api.ChatRequest, onEvent conversation.ChatResponseListener) error {
	return o.client.Chat(ctx, req, func(response api.ChatResponse) error {
		return onEvent.OnChatResponse(ctx, &response)
	})
}

func NewLLM(ctx context.Context, cfg *config.File) (conversation.LLM, error) {
	return NewLLMForModel(ctx, cfg, cfg.LanguageModel())
}

// NewLLMForModel creates a new LLM using the specified model override.
func NewLLMForModel(ctx context.Context, cfg *config.File, model string) (conversation.LLM, error) {
	switch cfg.Provider() {
	case config.ProviderGemini:
		return newGeminiLLMForModel(ctx, cfg, model)
	case config.ProviderOpenRouter:
		return newOpenRouterLLMForModel(cfg, model)
	case config.ProviderOllama:
		fallthrough
	default:
		return newOllamaLLMFromEnv()
	}
}

func newGeminiLLM(ctx context.Context, cfg *config.File) (conversation.LLM, error) {
	return newGeminiLLMForModel(ctx, cfg, cfg.LanguageModel())
}

func newGeminiLLMForModel(ctx context.Context, cfg *config.File, model string) (conversation.LLM, error) {
	apiKey, has, err := cfg.Gemini.ResolveKey()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Gemini API key: %w", err)
	}
	if !has {
		return nil, fmt.Errorf("gemini API key is required. Set it via config, file, or GEMINI_API_KEY env var")
	}

	return gemini.NewLLM(ctx, apiKey, model)
}

func newOpenRouterLLM(cfg *config.File) (conversation.LLM, error) {
	return newOpenRouterLLMForModel(cfg, cfg.LanguageModel())
}

func newOpenRouterLLMForModel(cfg *config.File, model string) (conversation.LLM, error) {
	apiKey, has, err := cfg.OpenRouter.ResolveKey()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve OpenRouter API key: %w", err)
	}
	if !has {
		return nil, fmt.Errorf("openrouter API key is required. Set it via config, file (--openrouter-key-file), or OPENROUTER_API_KEY env var")
	}

	baseURL := ""
	if cfg.OpenRouter != nil && cfg.OpenRouter.BaseURL != "" {
		baseURL = cfg.OpenRouter.BaseURL
	}

	return openrouter.NewLLM(apiKey, baseURL, model), nil
}

func newOllamaLLMFromEnv() (conversation.LLM, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create Ollama client: %w", err)
	}
	return NewOllamaLLM(client), nil
}
