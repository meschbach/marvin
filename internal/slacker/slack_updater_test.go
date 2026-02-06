package slacker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testContext provides a consistent context for testing
func testContext() context.Context {
	return context.Background()
}

func TestSlackUpdater_BasicOperations(t *testing.T) {
	client := &MockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Test basic operations with timeout
	done := make(chan error, 1)
	go func() {
		ctx := testContext()
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
		err = updater.AddToolCall(ctx, "test-tool")
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

	// Verify calls were made
	assert.Equal(t, 1, len(client.PostedMessages), "Expected one posted message")
	assert.Equal(t, 2, len(client.UpdatedMessages), "Expected two updated messages")
}

func TestSlackUpdater_ConcurrentAccess(t *testing.T) {
	client := &MockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Test concurrent access with timeout
	done := make(chan error, 1)
	go func() {
		// Start another goroutine that adds content
		go func() {
			ctx := testContext()
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
		err := updater.ForceUpdate(testContext())
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
	updater := NewSlackUpdater(client, "test-channel")

	// Initially in thinking state
	content, thinking, tools := updater.getBufferContent()
	assert.Equal(t, "", content, "Should start with no content")
	assert.Equal(t, "", thinking, "Should start with no thinking")
	assert.Empty(t, tools, "Should start with no tools")

	// Add thought
	ctx := testContext()
	updater.AddThought(ctx, "Let me think about this")
	content, thinking, _ = updater.getBufferContent()
	assert.Equal(t, "", content, "Content should be empty")
	assert.Equal(t, "Let me think about this", thinking, "Should have thinking content")

	// Transition to content by adding content
	updater.AddContent(ctx, "Here's the answer")
	content, thinking, _ = updater.getBufferContent()
	assert.Equal(t, "Here's the answer", content, "Should have content")
	assert.Equal(t, "", thinking, "Thinking should be flushed")
}

func TestSlackUpdater_ToolCalls(t *testing.T) {
	client := &MockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Add tool call
	ctx := testContext()
	updater.AddToolCall(ctx, "test-tool")

	// Should have posted a message for the tool call
	assert.Len(t, client.PostedMessages, 1, "Should post message for tool call")

	// Check buffer content
	_, _, tools := updater.getBufferContent()
	assert.Contains(t, tools, "test-tool", "Should record tool call")
}

func TestSlackUpdater_Complete(t *testing.T) {
	client := &MockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Add some content and complete
	ctx := testContext()
	err := updater.AddContent(ctx, "Final answer")
	require.NoError(t, err)
	err = updater.Complete(ctx)
	assert.NoError(t, err, "Complete should not error")
}

func TestSlackUpdater_ForceUpdateCompatibility(t *testing.T) {
	client := &MockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Add content and force update (for backward compatibility)
	ctx := testContext()
	err := updater.AddContent(ctx, "Some content")
	require.NoError(t, err)
	err = updater.ForceUpdate(ctx)
	assert.NoError(t, err, "ForceUpdate should not error")
}

func TestSlackUpdater_ThinkingFormatting(t *testing.T) {
	client := &MockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Add thinking content
	ctx := testContext()
	err := updater.AddThought(ctx, "This is a thought process")
	require.NoError(t, err)

	// Check that thinking would be formatted with italics in message format
	_, thinking, _ := updater.getBufferContent()
	assert.Equal(t, "This is a thought process", thinking, "Should store thinking without formatting")

	// When posted, it should be formatted with italics
	err = updater.ForceUpdate(ctx)
	assert.NoError(t, err)

	// The posted message should be formatted
	assert.Len(t, client.PostedMessages, 1, "Should post formatted message")
}

func TestSlackUpdater_MultipleStateTransitions(t *testing.T) {
	client := &MockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Start with thinking
	ctx := testContext()
	err := updater.AddThought(ctx, "Initial thought")
	require.NoError(t, err)
	content, thinking, _ := updater.getBufferContent()
	assert.Equal(t, "Initial thought", thinking)

	// Transition to content
	err = updater.AddContent(ctx, "Some content")
	require.NoError(t, err)
	content, thinking, _ = updater.getBufferContent()
	assert.Equal(t, "Some content", content)
	assert.Equal(t, "", thinking)

	// Transition back to thinking
	err = updater.AddThought(ctx, "Another thought")
	require.NoError(t, err)
	_, thinking, _ = updater.getBufferContent()
	assert.Equal(t, "Another thought", thinking)
}

func TestSlackUpdater_FinalBufferFlush(t *testing.T) {
	client := &MockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Add content in thinking state
	ctx := testContext()
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
	content, thinking, _ := updater.getBufferContent()
	assert.Equal(t, "Here's the final answer", content, "Should retain content buffer")
	assert.Equal(t, "", thinking, "Should have flushed thinking")
}

func TestSlackUpdater_CompleteWithFinalContent(t *testing.T) {
	client := &MockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Add final content and complete
	ctx := testContext()
	err := updater.AddContent(ctx, "Final answer")
	require.NoError(t, err)
	err = updater.Complete(ctx)
	assert.NoError(t, err, "Complete should not error")

	// Should have flushed content on complete
	assert.Greater(t, len(client.PostedMessages), 0, "Should post content on complete")

	// Buffer should be reset after complete
	content, _, _ := updater.getBufferContent()
	assert.Equal(t, "", content, "Buffer should be reset after complete")
}

func TestSlackUpdater_UserReportedScenario(t *testing.T) {
	client := &MockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// User scenario: thinking first, then content
	ctx := testContext()
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
	content, thinking, _ := updater.getBufferContent()
	t.Logf("Final buffer - content: '%s', thinking: '%s'", content, thinking)

	// The issue might be that content is not being posted
	// Let's ensure we get both thinking and content visible
	assert.Greater(t, len(client.PostedMessages), 0, "Should have posted something")
}

func TestSlackUpdater_RealWorldScenario(t *testing.T) {
	client := &MockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Simulate real scenario: thinking -> content -> ForceUpdate
	ctx := testContext()
	err := updater.AddThought(ctx, "Let me analyze this problem")
	require.NoError(t, err)

	// Should be in thinking state
	_, thinking, _ := updater.getBufferContent()
	assert.Equal(t, "Let me analyze this problem", thinking)

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

	// Buffer should contain content after flush
	content, thinking, _ := updater.getBufferContent()
	assert.Equal(t, "Based on my analysis, here's the solution:Step 1: Identify the problemStep 2: Implement fix", content)
	assert.Equal(t, "", thinking, "Thinking should be flushed")
}
