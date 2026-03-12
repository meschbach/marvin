package query

import (
	"testing"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLIStreamingUpdater_BasicFunctionality(t *testing.T) {
	t.Parallel()
	updater := NewCLIStreamingUpdater(true, true, true, "plain")
	ctx := t.Context()

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
	stats := conversation.Stats{
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
	t.Parallel()
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
			t.Parallel()
			updater := NewCLIStreamingUpdater(true, false, false, tt.thinkingFormat)
			ctx := t.Context()

			err := updater.AddThought(ctx, "test thinking")
			require.NoError(t, err)
		})
	}
}

func TestCLIStreamingUpdater_StatisticsTracking(t *testing.T) {
	t.Parallel()
	updater := NewCLIStreamingUpdater(false, false, false, "plain")
	ctx := t.Context()

	// Initial state
	assert.Equal(t, 0, updater.promptTokens)
	assert.Equal(t, 0, updater.responseTokens)
	assert.Equal(t, 0, updater.totalTokens)

	// Update statistics
	stats1 := conversation.Stats{
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
	stats2 := conversation.Stats{
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

func TestCLIStreamingUpdater_ToolsDisabled(t *testing.T) {
	t.Parallel()
	updater := NewCLIStreamingUpdater(false, false, false, "plain") // showTools=false
	ctx := t.Context()

	toolCall := api.ToolCall{
		Function: api.ToolCallFunction{
			Name: "test-tool",
		},
	}
	result := []api.Message{
		{Role: "assistant", Content: "result"},
	}

	// AddToolCall should return nil without printing
	err := updater.AddToolCall(ctx, toolCall)
	require.NoError(t, err)

	// AddToolResult should return nil without printing (success)
	err = updater.AddToolResult(ctx, toolCall, result, nil)
	require.NoError(t, err)

	// AddToolResult with error should also not print
	err = updater.AddToolResult(ctx, toolCall, nil, assert.AnError)
	require.NoError(t, err)

	// Flush should succeed
	err = updater.Flush(ctx)
	require.NoError(t, err)
}
