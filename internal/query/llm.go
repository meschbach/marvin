package query

import (
	"context"

	"github.com/ollama/ollama/api"
)

// LLM interface abstracts the underlying LLM client for testability and future provider support
type LLM interface {
	Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error
}

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
