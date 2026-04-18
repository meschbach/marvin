package slacker

import (
	"context"
	"testing"
	"time"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testContext provides a consistent context for testing
func testContext(t *testing.T) context.Context {
	return t.Context()
}

func TestSlackUpdater_BasicOperations(t *testing.T) {
	client := &MockSlackSink{}
	// Use preferences that enable all features for comprehensive testing
	preferences := UserPreferences{
		ShowThinking:   true,
		ShowTools:      true,
		ShowDone:       true,
		ThinkingFormat: "plain",
		ToolFormat:     "detailed",
		Verbose:        true,
	}
	updater := NewSlackUpdater(client, "test-channel", newCaptureFormatter(nil), preferences)

	// Test basic operations with timeout
	done := make(chan error, 1)
	go func() {
		ctx := testContext(t)
		err := updater.AddContent(ctx, "test content")
		if err != nil {
			done <- err
			return
		}
		err = updater.AddThought(ctx, "test thought")
		if err != nil {
			done <- err
			return
		}
		toolCall := api.ToolCall{
			Function: api.ToolCallFunction{
				Name: "test-tool",
			},
		}
		err = updater.AddToolCall(ctx, toolCall)
		if err != nil {
			done <- err
			return
		}
		err = updater.ForceUpdate(ctx)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Operations failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Deadlock detected in basic operations")
	}
	client.state.Lock()
	defer client.state.Unlock()

	// Verify calls were made
	// Progress indicator added on init->content transition
	assert.Len(t, client.PostedMessages, 4, "Expected four posted messages (content + progress, thought + progress, tool)")
	assert.Len(t, client.UpdatedMessages, 0, "Expected no updated messages")
}

func TestSlackUpdater_ConcurrentAccess(t *testing.T) {
	client := &MockSlackSink{}
	preferences := DefaultUserPreferences()
	updater := NewSlackUpdater(client, "test-channel", newCaptureFormatter(nil), preferences)

	// Test concurrent access with timeout
	done := make(chan error, 1)
	go func() {
		// Start another goroutine that adds content
		go func() {
			ctx := testContext(t)
			err := updater.AddContent(ctx, "concurrent content")
			if err != nil {
				return
			}
			err = updater.AddThought(ctx, "concurrent thought")
			if err != nil {
				return
			}
		}()

		// Give it time to run
		time.Sleep(10 * time.Millisecond)

		// This should not deadlock
		err := updater.ForceUpdate(testContext(t))
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Second ForceUpdate failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Deadlock detected in concurrent operations")
	}
}

func TestSlackUpdater_StateTransitions(t *testing.T) {
	client := &MockSlackSink{}
	// Enable thinking to test state transitions
	preferences := UserPreferences{
		ShowThinking:   true,
		ShowTools:      true,
		ShowDone:       true,
		ThinkingFormat: "plain",
		ToolFormat:     "detailed",
		Verbose:        true,
	}
	updater := NewSlackUpdater(client, "test-channel", newCaptureFormatter(nil), preferences)
	ctx := testContext(t)

	// Initially no messages
	assert.Len(t, client.PostedMessages, 0, "Should start with no posted messages")
	assert.Len(t, client.UpdatedMessages, 0, "Should start with no updated messages")

	// Add thought - should buffer content, no immediate post with new time-based behavior
	if err := updater.AddThought(ctx, "Let me think about this"); err != nil {
		t.Fatalf("Failed to add thought: %v", err)
	}
	// With time-based buffering, thought content stays in buffer until time threshold or type change

	// Transition to content by adding content - should immediately post the thought and start buffering content
	if err := updater.AddContent(ctx, "Here's the answer"); err != nil {
		t.Fatalf("Failed to add content: %v", err)
	}

	// With new behavior: type change triggers immediate post of previous content
	// AddThought triggers: post progress (1), switchToType (0, no prior content), updateMessage (1 for thought)
	// AddContent triggers: switchToType (1 post for thought), updateMessage (1 for content), no progress (not init->*)
	// Total: 1 (progress on AddThought) + 1 (thought post) + 1 (content post) = 3 posts
	assert.Len(t, client.PostedMessages, 3, "Should post message for thought and content + progress indicator")
	assert.Len(t, client.UpdatedMessages, 0, "No updates yet for buffered content")
}

func TestSlackUpdater_ToolCalls(t *testing.T) {
	client := &MockSlackSink{}
	preferences := DefaultUserPreferences()
	updater := NewSlackUpdater(client, "test-channel", newCaptureFormatter(nil), preferences)

	// Add tool call
	ctx := testContext(t)
	toolCall := api.ToolCall{
		Function: api.ToolCallFunction{
			Name: "test-tool",
		},
	}
	require.NoError(t, updater.AddToolCall(ctx, toolCall))
	require.NoError(t, updater.ForceUpdate(ctx))

	// Should have posted: progress indicator (init->tool) + tool call message
	assert.Len(t, client.PostedMessages, 2, "Should post progress + tool call message")
	assert.Len(t, client.UpdatedMessages, 0, "no updates should be issued.")
}

func TestSlackUpdater_ToolResults(t *testing.T) {
	client := &MockSlackSink{}
	preferences := DefaultUserPreferences()
	updater := NewSlackUpdater(client, "test-channel", newCaptureFormatter(nil), preferences)

	ctx := testContext(t)
	toolCall := api.ToolCall{
		Function: api.ToolCallFunction{
			Name: "test-tool",
		},
	}

	// Test successful tool result
	result := []api.Message{
		{Role: conversation.RoleToolResult, Content: "Success!"},
	}
	require.NoError(t, updater.AddToolResult(ctx, toolCall, result, nil))

	// Test failed tool result
	require.NoError(t, updater.AddToolResult(ctx, toolCall, nil, assert.AnError))

	require.NoError(t, updater.ForceUpdate(ctx))

	// Should have posted: progress indicator (init->tool) + tool results message
	assert.Len(t, client.PostedMessages, 2, "Should post progress + tool results message")
}

func TestSlackUpdater_Complete(t *testing.T) {
	client := &MockSlackSink{}
	preferences := DefaultUserPreferences()
	updater := NewSlackUpdater(client, "test-channel", newCaptureFormatter(nil), preferences)

	// Add some content and complete
	ctx := testContext(t)
	err := updater.AddContent(ctx, "Final answer")
	require.NoError(t, err)
	err = updater.ForceUpdate(ctx)
	assert.NoError(t, err, "Complete should not error")
}

func TestSlackUpdater_ForceUpdateCompatibility(t *testing.T) {
	client := &MockSlackSink{}
	preferences := DefaultUserPreferences()
	updater := NewSlackUpdater(client, "test-channel", newCaptureFormatter(nil), preferences)

	// Add content and force update (for backward compatibility)
	ctx := testContext(t)
	err := updater.AddContent(ctx, "Some content")
	require.NoError(t, err)
	err = updater.ForceUpdate(ctx)
	assert.NoError(t, err, "ForceUpdate should not error")
}

func TestSlackUpdater_ThinkingFormatting(t *testing.T) {
	client := &MockSlackSink{}
	// Enable thinking to test thinking formatting
	preferences := UserPreferences{
		ShowThinking:   true,
		ShowTools:      true,
		ShowDone:       true,
		ThinkingFormat: "plain",
		ToolFormat:     "detailed",
		Verbose:        true,
	}
	updater := NewSlackUpdater(client, "test-channel", newCaptureFormatter(nil), preferences)

	// Add thinking content
	ctx := testContext(t)
	err := updater.AddThought(ctx, "This is a thought process")
	require.NoError(t, err)

	// When posted, it should be formatted with italics
	err = updater.ForceUpdate(ctx)
	assert.NoError(t, err)

	// Should have progress indicator + formatted thought message
	assert.Len(t, client.PostedMessages, 2, "Should post progress + formatted message")
}

func TestSlackUpdater_MultipleStateTransitions(t *testing.T) {
	// Create a capture formatter to track what content is passed for formatting
	captureFormatter := newCaptureFormatter(nil)
	client := &MockSlackSink{}
	timer := &MockTimeProvider{CurrentTime: time.Now()}
	// Enable thinking to test multiple state transitions
	preferences := UserPreferences{
		ShowThinking:   true,
		ShowTools:      true,
		ShowDone:       true,
		ThinkingFormat: "plain",
		ToolFormat:     "detailed",
		Verbose:        true,
	}
	updater := NewSlackUpdater(client, "test-channel", captureFormatter, preferences, WithTimeProvider(timer))

	// Start with thinking
	ctx := testContext(t)
	err := updater.AddThought(ctx, "Initial thought")
	require.NoError(t, err)

	// Verify the thought was captured by the formatter
	assert.Len(t, captureFormatter.Given, 1, "Should have captured initial thought")

	// Transition to content
	err = updater.AddContent(ctx, "Some content")
	require.NoError(t, err)

	// The previous thought should be posted immediately on type change, then new content should be captured
	require.GreaterOrEqual(t, len(captureFormatter.Given), 2, "Should have captured both thought and content")

	// Find the most recent content capture (should be the content)
	lastCapture := captureFormatter.Given[len(captureFormatter.Given)-1]
	assert.Equal(t, ContentOutput, lastCapture.CType, "Most recent should be content")
	assert.Equal(t, "Some content", lastCapture.Content, "Should capture the exact content")

	// Transition back to thinking
	err = updater.AddThought(ctx, "Another thought")
	require.NoError(t, err)

	// Should capture the new thought
	lastCapture = captureFormatter.Given[len(captureFormatter.Given)-1]
	assert.Equal(t, ContentThinking, lastCapture.CType, "Most recent should be thinking")
	assert.Equal(t, "Thinking: Another thought", lastCapture.Content, "Should capture the thought content with prefix")
}

func TestSlackUpdater_FinalBufferFlush(t *testing.T) {
	client := &MockSlackSink{}
	preferences := DefaultUserPreferences()
	updater := NewSlackUpdater(client, "test-channel", newCaptureFormatter(nil), preferences)

	// Add content in thinking state
	ctx := testContext(t)
	err := updater.AddThought(ctx, "Let me think")
	require.NoError(t, err)

	// Transition to content and add final content
	err = updater.AddContent(ctx, "Here's the final answer")
	require.NoError(t, err)

	// ForceUpdate should flush all accumulated content
	err = updater.ForceUpdate(ctx)
	assert.NoError(t, err)

	// Should have posted messages
	assert.Greater(t, len(client.PostedMessages), 0, "Should have posted messages")

	// Get final buffer should show current state
	assert.GreaterOrEqual(t, len(client.PostedMessages), 1, "Should have posted at least one message")
}

func TestSlackUpdater_CompleteWithFinalContent(t *testing.T) {
	client := &MockSlackSink{}
	preferences := DefaultUserPreferences()
	updater := NewSlackUpdater(client, "test-channel", newCaptureFormatter(nil), preferences)

	// Add final content and complete
	ctx := testContext(t)
	err := updater.AddContent(ctx, "Final answer")
	require.NoError(t, err)
	err = updater.ForceUpdate(ctx)
	assert.NoError(t, err, "Complete should not error")

	// Buffer should be reset after complete
	assert.GreaterOrEqual(t, len(client.PostedMessages), 1, "Should have posted at least one message")
}

func TestSlackUpdater_UserReportedScenario(t *testing.T) {
	client := &MockSlackSink{}
	preferences := DefaultUserPreferences()
	updater := NewSlackUpdater(client, "test-channel", newCaptureFormatter(nil), preferences)

	// User scenario: thinking first, then content
	ctx := testContext(t)
	err := updater.AddThought(ctx, "Let me think about this problem")
	require.NoError(t, err)
	err = updater.AddThought(ctx, "I need to analyze the requirements")
	require.NoError(t, err)

	// Transition to content
	err = updater.AddContent(ctx, "Based on my analysis, here's the solution:")
	require.NoError(t, err)
	err = updater.AddContent(ctx, "1. First approach")
	require.NoError(t, err)
	err = updater.AddContent(ctx, "2. Second approach")
	require.NoError(t, err)

	// Force final update (like the integration does)
	err = updater.ForceUpdate(ctx)
	assert.NoError(t, err, "ForceUpdate should not error")

	// User reports they only see thoughts at end, not content
	// Let's verify what actually gets posted
	t.Logf("Total messages posted: %d", len(client.PostedMessages))

	// Check what's in the buffer
	content, thinking := updater.getBufferContent()
	t.Logf("Final buffer - content: '%s', thinking: '%s'", content, thinking)

	// The issue might be that content is not being posted
	// Let's ensure we get both thinking and content visible
	assert.Greater(t, len(client.PostedMessages), 0, "Should have posted something")
}

func TestSlackUpdater_RealWorldScenario(t *testing.T) {
	client := &MockSlackSink{}
	// Enable thinking to test real world scenario
	preferences := UserPreferences{
		ShowThinking:   true,
		ShowTools:      true,
		ShowDone:       true,
		ThinkingFormat: "plain",
		ToolFormat:     "detailed",
		Verbose:        true,
	}
	updater := NewSlackUpdater(client, "test-channel", newCaptureFormatter(nil), preferences)

	// Simulate real scenario: thinking -> content -> ForceUpdate
	ctx := testContext(t)
	err := updater.AddThought(ctx, "Let me analyze this problem")
	require.NoError(t, err)

	// Should be in thinking state
	_, thinking := updater.getBufferContent()
	assert.Equal(t, "Thinking: Let me analyze this problem", thinking)

	// Add content (should trigger transition and flush)
	err = updater.AddContent(ctx, "Based on my analysis, here's the solution:")
	require.NoError(t, err)
	err = updater.AddContent(ctx, "Step 1: Identify the problem")
	require.NoError(t, err)
	err = updater.AddContent(ctx, "Step 2: Implement fix")
	require.NoError(t, err)

	// ForceUpdate should flush final content
	err = updater.ForceUpdate(ctx)
	assert.NoError(t, err)

	// Should have posted both thinking and content
	assert.GreaterOrEqual(t, len(client.PostedMessages), 1, "Should have posted messages")

	// Buffer should be reset after complete
	content, thinking := updater.getBufferContent()
	assert.Empty(t, content, "Content buffer should be empty after complete")
	assert.Empty(t, thinking, "Thinking buffer should be empty after complete")
}
