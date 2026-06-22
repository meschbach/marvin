package factory

import (
	"context"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/llm"
	"github.com/meschbach/marvin/internal/llm/gemini"
	"github.com/meschbach/marvin/internal/llm/ollama"
	"github.com/meschbach/marvin/internal/llm/openrouter"
)

// NewFromConfig creates an LLM chain from configuration.
// It builds the appropriate provider chain based on the config mode (legacy or structured).
func NewFromConfig(ctx context.Context, cfg *config.File, accessCheck func(string) bool) (llm.LLM, error) {
	chainCfg := llm.Config{
		File:        cfg,
		AccessCheck: accessCheck,
	}

	return llm.NewChain(ctx, chainCfg, func(provider, model string) (llm.LLM, error) {
		return createProvider(ctx, cfg, provider, model)
	})
}

// NewFromConfigForModel creates a single LLM provider for the specified model.
// Used when a specific model override is needed (e.g., goal command).
func NewFromConfigForModel(ctx context.Context, cfg *config.File, model string) (llm.LLM, error) {
	return createProvider(ctx, cfg, string(cfg.Provider()), model)
}

func createProvider(ctx context.Context, cfg *config.File, providerName, model string) (llm.LLM, error) {
	switch config.ProviderType(providerName) {
	case config.ProviderGemini:
		return newGeminiLLMForModel(ctx, cfg, model)
	case config.ProviderOpenRouter:
		return newOpenRouterLLMForModel(cfg, model)
	case config.ProviderOllama:
		return newOllamaLLMFromEnv()
	default:
		return newOllamaLLMFromEnv()
	}
}

func newGeminiLLMForModel(ctx context.Context, cfg *config.File, model string) (llm.LLM, error) {
	if cfg.Gemini == nil {
		return nil, fmt.Errorf("gemini configuration required")
	}
	apiKey, has, err := cfg.Gemini.ResolveKey()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Gemini API key: %w", err)
	}
	if !has {
		return nil, fmt.Errorf("gemini API key is required. Set it via config, file, or GEMINI_API_KEY env var")
	}

	return gemini.NewLLM(ctx, apiKey, model)
}

func newOpenRouterLLMForModel(cfg *config.File, model string) (llm.LLM, error) {
	if cfg.OpenRouter == nil {
		return nil, fmt.Errorf("openrouter configuration required")
	}
	apiKey, has, err := cfg.OpenRouter.ResolveKey()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve OpenRouter API key: %w", err)
	}
	if !has {
		return nil, fmt.Errorf("openrouter API key is required. Set it via config, file (--openrouter-key-file), or OPENROUTER_API_KEY env var")
	}

	baseURL := ""
	if cfg.OpenRouter.BaseURL != "" {
		baseURL = cfg.OpenRouter.BaseURL
	}

	var retryConfig *config.RetryBlock
	if cfg.OpenRouter != nil {
		retryConfig = cfg.OpenRouter.Retry
	}

	return openrouter.NewLLM(apiKey, baseURL, model, retryConfig), nil
}

func newOllamaLLMFromEnv() (llm.LLM, error) {
	return ollama.NewProviderFromEnv()
}

// NewEmbeddingProvider creates an embedding provider from configuration.
// Currently only Ollama supports embeddings.
func NewEmbeddingProvider(ctx context.Context, cfg *config.File) (llm.EmbeddingProvider, error) {
	return ollama.NewProviderFromEnv()
}
