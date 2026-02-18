package conversation

import (
	"context"

	"github.com/ollama/ollama/api"
)

// OneShotLLM provides a set of chat responses all at once
type OneShotLLM struct {
	responses []api.ChatResponse
}

func (m *OneShotLLM) Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	//nolint:gocritic
	for _, resp := range m.responses {
		if err := fn(resp); err != nil {
			return err
		}
	}
	return nil
}
