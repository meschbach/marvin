package conversation

import (
	"context"

	"github.com/meschbach/marvin/internal/llm"
)

// OneShotLLM provides a set of chat responses all at once
type OneShotLLM struct {
	responses []llm.ChatResponse
}

func (m *OneShotLLM) Chat(ctx context.Context, _ *llm.ChatRequest, onResponse func(ctx context.Context, resp *llm.ChatResponse) error) error {
	for i := range m.responses {
		if err := onResponse(ctx, &m.responses[i]); err != nil {
			return err
		}
	}
	return nil
}
