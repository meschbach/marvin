package openrouter

import (
	openrouter "github.com/revrost/go-openrouter"
)

const defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

type LLM struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *openrouter.Client
}

func NewLLM(apiKey, baseURL, model string) *LLM {
	if baseURL == "" {
		baseURL = defaultOpenRouterBaseURL
	}

	config := openrouter.DefaultConfig(apiKey)
	config.BaseURL = baseURL
	config.HttpReferer = "https://github.com/meschbach/marvin"
	config.XTitle = "Marvin"

	client := openrouter.NewClientWithConfig(*config)

	return &LLM{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: client,
	}
}
