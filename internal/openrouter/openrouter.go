package openrouter

import (
	"encoding/json"
	"net/http"

	"github.com/ollama/ollama/api"
)

const defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

type LLM struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

type Request struct {
	Model       string          `json:"model"`
	Messages    []api.Message   `json:"messages"`
	Tools       json.RawMessage `json:"tools,omitempty"`
	Stream      bool            `json:"stream"`
	Temperature *float32        `json:"temperature,omitempty"`
	TopP        *float32        `json:"top_p,omitempty"`
	TopK        *int            `json:"top_k,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Seed        *int            `json:"seed,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
}

type Response struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Message      api.Message `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

type StreamResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Delta        api.Message `json:"delta"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func NewLLM(apiKey, baseURL, model string) *LLM {
	if baseURL == "" {
		baseURL = defaultOpenRouterBaseURL
	}
	return &LLM{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{},
	}
}
