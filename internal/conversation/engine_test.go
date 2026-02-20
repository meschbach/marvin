package conversation

import (
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConversationEngine_ContentBeforeDone(t *testing.T) {
	t.Parallel()
	mockLLM := &OneShotLLM{
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

	updater := &TrackingUpdater{}
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

	err := engine.RunConversation(t.Context(), "test-model", updater)
	require.NoError(t, err)

	assert.True(t, updater.lastStats.IsDone, "should be done")
	assert.Equal(t, 10, updater.lastStats.EvalCount)

	contentIdx := -1
	doneIdx := -1
	for i, event := range updater.events {
		// we don't care about the remainder types
		// nolint
		switch event.Kind {
		case TrackerEventContent:
			contentIdx = i
		case TrackerEventFlush:
			doneIdx = i
		}
	}

	assert.GreaterOrEqual(t, contentIdx, 0, "should have content event")
	assert.GreaterOrEqual(t, doneIdx, 0, "should have done event")
	assert.Less(t, contentIdx, doneIdx,
		"content should be received before done. Got events: %v", updater.events)
}

func TestConversationEngine_ContentWithDoneInSameChunk(t *testing.T) {
	t.Parallel()
	mockLLM := &OneShotLLM{
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

	updater := &TrackingUpdater{}
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

	err := engine.RunConversation(t.Context(), "test-model", updater)
	require.NoError(t, err)

	assert.True(t, updater.lastStats.IsDone)
	assert.Equal(t, 8, updater.lastStats.EvalCount)

	contentIdx := -1
	doneIdx := -1
	for i, event := range updater.events {
		if event.Kind == TrackerEventContent {
			contentIdx = i
		}
		if event.Kind == TrackerEventFlush {
			doneIdx = i
		}
	}

	assert.GreaterOrEqual(t, contentIdx, 0, "should have content event")
	assert.GreaterOrEqual(t, doneIdx, 0, "should have done event")
	assert.Less(t, contentIdx, doneIdx,
		"content should be received before done. Got events: %v", updater.events)
}

func TestConversationEngine_ThinkingBeforeDone(t *testing.T) {
	t.Parallel()

	paragraph := faker.Paragraph()
	mockLLM := &OneShotLLM{
		responses: []api.ChatResponse{
			{
				Model: "test-model",
				Message: api.Message{
					Role:     "assistant",
					Content:  "",
					Thinking: paragraph,
				},
				Done: true,
				Metrics: api.Metrics{
					EvalCount:       15,
					PromptEvalCount: 10,
				},
			},
		},
	}

	updater := &TrackingUpdater{}
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

	err := engine.RunConversation(t.Context(), "test-model", updater)
	require.NoError(t, err)

	assert.True(t, updater.lastStats.IsDone)

	thoughts := updater.Thoughts()
	if assert.Len(t, thoughts, 1) {
		assert.Equal(t, thoughts[0].Value, paragraph)
	}
	// verify the flush is last
	assert.Equal(t, TrackerEventFlush, updater.Last().Kind)
}
