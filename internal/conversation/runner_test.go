package conversation

import (
	"context"
	"fmt"
	"testing"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLLMCreator is a test double for LLMCreator.
type mockLLMCreator struct {
	llm LLM
	err error
}

func (m *mockLLMCreator) Create(_ context.Context, _ *config.File, _ string) (LLM, error) {
	return m.llm, m.err
}

// mockToolProvider is a test double for ToolProvider.
type mockToolProvider struct {
	toolSet *ToolSet
	err     error
}

func (m *mockToolProvider) Provide(_ context.Context, _ *config.File, _ string, _ []string) (*ToolSet, error) {
	return m.toolSet, m.err
}

// TestRunner_Run_BasicExecution tests that Runner.Run successfully executes a simple conversation.
func TestRunner_Run_BasicExecution(t *testing.T) {
	t.Parallel()

	// Setup: one-shot LLM that returns a simple response
	llm := &OneShotLLM{
		responses: []api.ChatResponse{
			{
				Model: "test-model",
				Message: api.Message{
					Role:    "assistant",
					Content: "Hello from runner",
				},
				Done: true,
				Metrics: api.Metrics{
					EvalCount:       5,
					PromptEvalCount: 3,
				},
			},
		},
	}

	llmCreator := &mockLLMCreator{llm: llm}
	toolProvider := &mockToolProvider{toolSet: NewToolSet()}

	runner := NewRunner(
		&config.File{Model: "test-model"},
		llmCreator.Create,
		toolProvider.Provide,
	)

	req := &RunRequest{
		UserID:       "user-1",
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []api.Message{},
		UserMessage:  "Hi",
		ToolNames:    []string{},
		Updater:      &TrackingUpdater{},
	}

	result, err := runner.Run(t.Context(), "test-model", req)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Content, "non-recording updater should return empty content")
}

// TestRunner_Run_WithRecordingUpdater tests that RecordingUpdater captures content.
func TestRunner_Run_WithRecordingUpdater(t *testing.T) {
	t.Parallel()

	llm := &OneShotLLM{
		responses: []api.ChatResponse{
			{
				Model: "test-model",
				Message: api.Message{
					Role:    "assistant",
					Content: "Recorded response",
				},
				Done: true,
			},
		},
	}

	llmCreator := &mockLLMCreator{llm: llm}
	toolProvider := &mockToolProvider{toolSet: NewToolSet()}
	recorder := NewRecordingUpdater()

	runner := NewRunner(
		&config.File{Model: "test-model"},
		llmCreator.Create,
		toolProvider.Provide,
	)

	req := &RunRequest{
		UserID:       "user-1",
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []api.Message{},
		UserMessage:  "Hi",
		ToolNames:    []string{},
		Updater:      recorder,
	}

	result, err := runner.Run(t.Context(), "test-model", req)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "Recorded response", result.Content)
	assert.Equal(t, "Recorded response", recorder.Content())
}

// TestRunner_Run_CallbackInvoked tests that the callback is invoked if provided.
func TestRunner_Run_CallbackInvoked(t *testing.T) {
	t.Parallel()

	llm := &OneShotLLM{
		responses: []api.ChatResponse{
			{
				Model: "test-model",
				Message: api.Message{
					Role:    "assistant",
					Content: "Callback test",
				},
				Done: true,
			},
		},
	}

	llmCreator := &mockLLMCreator{llm: llm}
	toolProvider := &mockToolProvider{toolSet: NewToolSet()}

	callbackInvoked := false
	callbackMessages := []api.Message{}
	callback := func(_ context.Context, msg api.Message) error {
		callbackInvoked = true
		callbackMessages = append(callbackMessages, msg)
		return nil
	}

	runner := NewRunner(
		&config.File{Model: "test-model"},
		llmCreator.Create,
		toolProvider.Provide,
	)

	req := &RunRequest{
		UserID:       "user-1",
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []api.Message{},
		UserMessage:  "Hi",
		ToolNames:    []string{},
		Updater:      &TrackingUpdater{},
		Callback:     callback,
	}

	_, err := runner.Run(t.Context(), "test-model", req)
	require.NoError(t, err)
	assert.True(t, callbackInvoked, "callback should be invoked")
	assert.Len(t, callbackMessages, 1, "callback should receive one message")
	assert.Equal(t, "Callback test", callbackMessages[0].Content)
}

// TestRunner_Run_LLMCreationFailure tests that Runner.Run returns an error if LLMCreator fails.
func TestRunner_Run_LLMCreationFailure(t *testing.T) {
	t.Parallel()

	llmCreator := &mockLLMCreator{err: fmt.Errorf("LLM unavailable")}
	toolProvider := &mockToolProvider{toolSet: NewToolSet()}

	runner := NewRunner(
		&config.File{Model: "test-model"},
		llmCreator.Create,
		toolProvider.Provide,
	)

	req := &RunRequest{
		UserID:       "user-1",
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []api.Message{},
		UserMessage:  "Hi",
		ToolNames:    []string{},
		Updater:      &TrackingUpdater{},
	}

	_, err := runner.Run(t.Context(), "test-model", req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating LLM")
	assert.Contains(t, err.Error(), "LLM unavailable")
}

// TestRunner_Run_ToolProviderFailure tests that Runner.Run returns an error if ToolProvider fails.
func TestRunner_Run_ToolProviderFailure(t *testing.T) {
	t.Parallel()

	llm := &OneShotLLM{
		responses: []api.ChatResponse{
			{
				Model:   "test-model",
				Message: api.Message{Role: "assistant", Content: "Hi"},
				Done:    true,
			},
		},
	}

	llmCreator := &mockLLMCreator{llm: llm}
	toolProvider := &mockToolProvider{err: fmt.Errorf("tool setup failed")}

	runner := NewRunner(
		&config.File{Model: "test-model"},
		llmCreator.Create,
		toolProvider.Provide,
	)

	req := &RunRequest{
		UserID:       "user-1",
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []api.Message{},
		UserMessage:  "Hi",
		ToolNames:    []string{},
		Updater:      &TrackingUpdater{},
	}

	_, err := runner.Run(t.Context(), "test-model", req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating tools")
	assert.Contains(t, err.Error(), "tool setup failed")
}

// TestRunner_Run_EngineFailure tests that Runner.Run returns an error if engine.RunConversation fails.
func TestRunner_Run_EngineFailure(t *testing.T) {
	t.Parallel()

	// Mock LLM that always returns an error
	failingLLM := &failingLLM{err: fmt.Errorf("LLM connection lost")}

	llmCreator := &mockLLMCreator{llm: failingLLM}
	toolProvider := &mockToolProvider{toolSet: NewToolSet()}

	runner := NewRunner(
		&config.File{Model: "test-model"},
		llmCreator.Create,
		toolProvider.Provide,
	)

	req := &RunRequest{
		UserID:       "user-1",
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []api.Message{},
		UserMessage:  "Hi",
		ToolNames:    []string{},
		Updater:      &TrackingUpdater{},
	}

	_, err := runner.Run(t.Context(), "test-model", req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "running conversation")
	assert.Contains(t, err.Error(), "LLM connection failed")
}

// failingLLM is a mock LLM that always fails.
type failingLLM struct{ err error }

func (f *failingLLM) Chat(_ context.Context, _ *api.ChatRequest, _ ChatResponseListener) error {
	return f.err
}
