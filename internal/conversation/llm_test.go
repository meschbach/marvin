package conversation

import (
	"context"

	"github.com/ollama/ollama/api"
)

// OneShotLLM provides a set of chat responses all at once
type OneShotLLM struct {
	responses []api.ChatResponse
}

func (m *OneShotLLM) Chat(ctx context.Context, _ *api.ChatRequest, onEvent ChatResponseListener) error {
	for i := range m.responses {
		if err := onEvent.OnChatResponse(ctx, &m.responses[i]); err != nil {
			return err
		}
	}
	return nil
}
