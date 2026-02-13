package query

import (
	"context"
	"testing"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLIStreamingUpdater_BasicFunctionality(t *testing.T) {
	updater := NewCLIStreamingUpdater(true, true, true, "plain")
	ctx := context.Background()

	// Test AddContent
	err := updater.AddContent(ctx, "Hello world")
	require.NoError(t, err)

	// Test AddThought
	err = updater.AddThought(ctx, "Let me think")
	require.NoError(t, err)

	// Test AddToolCall
	toolCall := api.ToolCall{
		Function: api.ToolCallFunction{
			Name: "calculator",
		},
	}
	err = updater.AddToolCall(ctx, toolCall)
	require.NoError(t, err)

	// Test UpdateStats
	stats := conversation.ConversationStats{
		PromptTokens:   10,
		ResponseTokens: 20,
		TotalTokens:    30,
		EvalCount:      1,
		DoneReason:     "stop",
		IsDone:         true,
	}
	err = updater.UpdateStats(ctx, stats)
	require.NoError(t, err)

	// Test AddToolResult - success case
	toolResult := []api.Message{
		{Role: conversation.RoleToolResult, Content: "42"},
	}
	err = updater.AddToolResult(ctx, toolCall, toolResult, nil)
	require.NoError(t, err)

	// Test AddToolResult - error case
	err = updater.AddToolResult(ctx, toolCall, nil, assert.AnError)
	require.NoError(t, err)

	// Test Flush
	err = updater.Flush(ctx)
	require.NoError(t, err)
}

func TestCLIStreamingUpdater_ThinkingFormats(t *testing.T) {
	tests := []struct {
		name           string
		thinkingFormat string
		expectedPrefix string
	}{
		{"plain format", "plain", "Thinking: "},
		{"markdown format", "markdown", "## 🤔 Thinking\n"},
		{"collapsed format", "collapsed", "🤔 Thinking: "},
		{"default format", "", "Thinking: "}, // defaults to plain
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updater := NewCLIStreamingUpdater(true, false, false, tt.thinkingFormat)
			ctx := context.Background()

			err := updater.AddThought(ctx, "test thinking")
			require.NoError(t, err)
		})
	}
}

func TestCLIStreamingUpdater_StatisticsTracking(t *testing.T) {
	updater := NewCLIStreamingUpdater(false, false, false, "plain")
	ctx := context.Background()

	// Initial state
	assert.Equal(t, 0, updater.promptTokens)
	assert.Equal(t, 0, updater.responseTokens)
	assert.Equal(t, 0, updater.totalTokens)

	// Update statistics
	stats1 := conversation.ConversationStats{
		PromptTokens:   5,
		ResponseTokens: 10,
		TotalTokens:    15,
		IsDone:         true,
	}
	err := updater.UpdateStats(ctx, stats1)
	require.NoError(t, err)

	// Check that statistics are tracked
	assert.Equal(t, 5, updater.promptTokens)
	assert.Equal(t, 10, updater.responseTokens)
	assert.Equal(t, 15, updater.totalTokens)

	// Update with cumulative stats
	stats2 := conversation.ConversationStats{
		PromptTokens:   8,
		ResponseTokens: 15,
		TotalTokens:    23,
		IsDone:         true,
	}
	err = updater.UpdateStats(ctx, stats2)
	require.NoError(t, err)

	// Check that statistics are updated
	assert.Equal(t, 8, updater.promptTokens)
	assert.Equal(t, 15, updater.responseTokens)
	assert.Equal(t, 23, updater.totalTokens)
}
