package conversation

import (
	"context"
	"strings"
	"testing"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLLMWithDoneContent struct {
	responses []api.ChatResponse
}

func (m *mockLLMWithDoneContent) Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	for _, resp := range m.responses {
		if err := fn(resp); err != nil {
			return err
		}
	}
	return nil
}

type trackingUpdater struct {
	events    []string
	showDone  bool
	lastStats ConversationStats
}

func (t *trackingUpdater) AddContent(ctx context.Context, content string) error {
	t.events = append(t.events, "content:"+content)
	return nil
}

func (t *trackingUpdater) AddThought(ctx context.Context, thought string) error {
	t.events = append(t.events, "thought:"+thought)
	return nil
}

func (t *trackingUpdater) AddToolCall(ctx context.Context, toolCall api.ToolCall) error {
	t.events = append(t.events, "toolcall:"+toolCall.Function.Name)
	return nil
}

func (t *trackingUpdater) AddToolResult(ctx context.Context, toolCall api.ToolCall, result []api.Message, err error) error {
	if err != nil {
		t.events = append(t.events, "toolresult_error:"+toolCall.Function.Name)
	} else {
		t.events = append(t.events, "toolresult:"+toolCall.Function.Name)
	}
	return nil
}

func (t *trackingUpdater) UpdateStats(ctx context.Context, stats ConversationStats) error {
	t.lastStats = stats
	if stats.IsDone && t.showDone {
		t.events = append(t.events, "done")
	}
	return nil
}

func (t *trackingUpdater) Flush(ctx context.Context) error {
	t.events = append(t.events, "flush")
	return nil
}

func TestConversationEngine_ContentBeforeDone(t *testing.T) {
	mockLLM := &mockLLMWithDoneContent{
		responses: []api.ChatResponse{
			{
				Model: "test-model",
				Message: api.Message{
					Role:    "assistant",
					Content: "Hello",
				},
				Done: false,
			},
			{
				Model: "test-model",
				Message: api.Message{
					Role:    "assistant",
					Content: " world",
				},
				Done: true,
				Metrics: api.Metrics{
					EvalCount:       10,
					PromptEvalCount: 5,
				},
			},
		},
	}

	updater := &trackingUpdater{showDone: true}
	cfg := &config.File{
		Model: "test-model",
	}
	engine := NewEngine(
		mockLLM,
		cfg,
		&NullLogger{},
		&ToolSet{},
		[]api.Message{{Role: "user", Content: "hi"}},
	)

	err := engine.RunConversation(context.Background(), "test-model", updater)
	require.NoError(t, err)

	assert.True(t, updater.lastStats.IsDone, "should be done")
	assert.Equal(t, 10, updater.lastStats.EvalCount)

	contentIdx := -1
	doneIdx := -1
	for i, event := range updater.events {
		if strings.HasPrefix(event, "content:") {
			contentIdx = i
		}
		if event == "done" {
			doneIdx = i
		}
	}

	assert.GreaterOrEqual(t, contentIdx, 0, "should have content event")
	assert.GreaterOrEqual(t, doneIdx, 0, "should have done event")
	assert.True(t, contentIdx < doneIdx,
		"content should be received before done. Got events: %v", updater.events)
}

func TestConversationEngine_ContentWithDoneInSameChunk(t *testing.T) {
	mockLLM := &mockLLMWithDoneContent{
		responses: []api.ChatResponse{
			{
				Model: "test-model",
				Message: api.Message{
					Role:    "assistant",
					Content: "Hi there!",
				},
				Done: true,
				Metrics: api.Metrics{
					EvalCount:       8,
					PromptEvalCount: 4,
				},
			},
		},
	}

	updater := &trackingUpdater{showDone: true}
	cfg := &config.File{
		Model: "test-model",
	}
	engine := NewEngine(
		mockLLM,
		cfg,
		&NullLogger{},
		&ToolSet{},
		[]api.Message{{Role: "user", Content: "hello"}},
	)

	err := engine.RunConversation(context.Background(), "test-model", updater)
	require.NoError(t, err)

	assert.True(t, updater.lastStats.IsDone)
	assert.Equal(t, 8, updater.lastStats.EvalCount)

	contentIdx := -1
	doneIdx := -1
	for i, event := range updater.events {
		if strings.HasPrefix(event, "content:") {
			contentIdx = i
		}
		if event == "done" {
			doneIdx = i
		}
	}

	assert.GreaterOrEqual(t, contentIdx, 0, "should have content event")
	assert.GreaterOrEqual(t, doneIdx, 0, "should have done event")
	assert.True(t, contentIdx < doneIdx,
		"content should be received before done. Got events: %v", updater.events)
}

func TestConversationEngine_ThinkingBeforeDone(t *testing.T) {
	mockLLM := &mockLLMWithDoneContent{
		responses: []api.ChatResponse{
			{
				Model: "test-model",
				Message: api.Message{
					Role:     "assistant",
					Content:  "",
					Thinking: "Let me think about this.",
				},
				Done: true,
				Metrics: api.Metrics{
					EvalCount:       15,
					PromptEvalCount: 10,
				},
			},
		},
	}

	updater := &trackingUpdater{showDone: true}
	cfg := &config.File{
		Model: "test-model",
	}
	engine := NewEngine(
		mockLLM,
		cfg,
		&NullLogger{},
		&ToolSet{},
		[]api.Message{{Role: "user", Content: "think"}},
	)

	err := engine.RunConversation(context.Background(), "test-model", updater)
	require.NoError(t, err)

	assert.True(t, updater.lastStats.IsDone)

	thoughtIdx := -1
	doneIdx := -1
	for i, event := range updater.events {
		if strings.HasPrefix(event, "thought:") {
			thoughtIdx = i
		}
		if event == "done" {
			doneIdx = i
		}
	}

	assert.GreaterOrEqual(t, thoughtIdx, 0, "should have thought event")
	assert.GreaterOrEqual(t, doneIdx, 0, "should have done event")
	assert.True(t, thoughtIdx < doneIdx,
		"thinking should be received before done. Got events: %v", updater.events)
}
