package slacker

import (
	"context"

	"github.com/ollama/ollama/api"
)

// SlackLLMAdapter adapts the generic LLM interface to work with existing Slack types
type SlackLLMAdapter[LLM llm] struct {
	languageService LLM
}

// NewSlackLLMAdapter creates a new adapter that wraps the Slack LLM service
func NewSlackLLMAdapter[LLM llm](languageService LLM) *SlackLLMAdapter[LLM] {
	return &SlackLLMAdapter[LLM]{
		languageService: languageService,
	}
}

// Chat implements the query.LLM interface by delegating to the Slack LLM service
func (s *SlackLLMAdapter[LLM]) Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	return s.languageService.Chat(ctx, req, fn)
}
