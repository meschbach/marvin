package llm

import (
	"context"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
)

// Config holds configuration for creating an LLM chain.
type Config struct {
	File        *config.File
	AccessCheck func(label string) bool
}

// ProviderFactory creates an LLM provider for a given provider name and model.
type ProviderFactory func(provider, model string) (LLM, error)

// NewChain builds an LLM chain from configuration using the provided factory.
// In legacy mode (no provider_model blocks), returns a single-entry chain.
// In structured mode, returns an OrderedChain with all models in preference order.
func NewChain(ctx context.Context, cfg Config, createProvider ProviderFactory) (LLM, error) {
	if cfg.File.IsLegacyMode() {
		return newLegacyChain(ctx, cfg, createProvider)
	}
	return newStructuredChain(ctx, cfg, createProvider)
}

func newLegacyChain(ctx context.Context, cfg Config, createProvider ProviderFactory) (LLM, error) {
	model := cfg.File.LanguageModel()
	provider, err := createProvider(string(cfg.File.Provider()), model)
	if err != nil {
		return nil, err
	}

	entry := ModelEntry{
		Label:    model,
		Provider: string(cfg.File.Provider()),
		LLM:      provider,
		Breaker:  NewRampingBreaker(model),
	}

	return NewOrderedChain(ctx, []ModelEntry{entry}, cfg.AccessCheck)
}

func newStructuredChain(ctx context.Context, cfg Config, createProvider ProviderFactory) (LLM, error) {
	providerCache := make(map[string]LLM)
	entries := make([]ModelEntry, 0, len(cfg.File.ProviderModels))

	for _, pm := range cfg.File.ProviderModels {
		provider, err := getOrCreateProvider(pm.Provider, pm.Model, providerCache, createProvider)
		if err != nil {
			return nil, fmt.Errorf("provider_model %q: %w", pm.Name, err)
		}

		entries = append(entries, ModelEntry{
			Label:    pm.Name,
			Provider: pm.Provider,
			LLM:      provider,
			Breaker:  NewRampingBreaker(pm.Name),
		})
	}

	if cfg.File.LLM != nil && len(cfg.File.LLM.Models) > 0 {
		ordered := make([]ModelEntry, 0, len(cfg.File.LLM.Models))
		entryMap := make(map[string]ModelEntry)
		for _, e := range entries {
			entryMap[e.Label] = e
		}
		for _, label := range cfg.File.LLM.Models {
			if e, ok := entryMap[label]; ok {
				ordered = append(ordered, e)
			}
		}
		entries = ordered
	}

	return NewOrderedChain(ctx, entries, cfg.AccessCheck)
}

func getOrCreateProvider(providerName, model string, cache map[string]LLM, createProvider ProviderFactory) (LLM, error) {
	if p, ok := cache[providerName]; ok {
		return p, nil
	}

	p, err := createProvider(providerName, model)
	if err != nil {
		return nil, err
	}
	cache[providerName] = p
	return p, nil
}
