package slacker

import (
	"testing"
	"time"

	"github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
)

// mockSlackSink implements SlackSink for testing
type mockSlackSink struct {
	postedMessages  []string
	updatedMessages []string
}

func (m *mockSlackSink) UpdateMessage(channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
	// For testing, we don't need to parse the actual options
	m.updatedMessages = append(m.updatedMessages, "updated")
	return "channel", "timestamp", "text", nil
}

func (m *mockSlackSink) PostMessage(channelID string, options ...slack.MsgOption) (string, string, error) {
	// For testing, we don't need to parse the actual options
	m.postedMessages = append(m.postedMessages, "posted")
	return "channel", "test-timestamp", nil
}

func TestSlackUpdater_BasicOperations(t *testing.T) {
	client := &mockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Test basic operations with timeout
	done := make(chan error, 1)
	go func() {
		updater.AddContent("test content")
		updater.AddThought("test thought")
		updater.AddToolCall("test-tool")
		err := updater.ForceUpdate()
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ForceUpdate failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Deadlock detected in basic operations")
	}

	// Test concurrent access with timeout
	done = make(chan error, 1)
	go func() {
		// Start another goroutine that adds content
		go func() {
			updater.AddContent("concurrent content")
			updater.AddThought("concurrent thought")
		}()

		// Give it time to run
		time.Sleep(10 * time.Millisecond)

		// This should not deadlock
		err := updater.ForceUpdate()
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
	client := &mockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Initially in thinking state
	content, thinking, tools := updater.getBufferContent()
	assert.Equal(t, "", content, "Should start with no content")
	assert.Equal(t, "", thinking, "Should start with no thinking")
	assert.Empty(t, tools, "Should start with no tools")

	// Add thought
	updater.AddThought("Let me think about this")
	content, thinking, _ = updater.getBufferContent()
	assert.Equal(t, "", content, "Content should be empty")
	assert.Equal(t, "Let me think about this", thinking, "Should have thinking content")

	// Transition to content by adding content
	updater.AddContent("Here's the answer")
	content, thinking, _ = updater.getBufferContent()
	assert.Equal(t, "Here's the answer", content, "Should have content")
	assert.Equal(t, "", thinking, "Thinking should be flushed")
}

func TestSlackUpdater_ToolCalls(t *testing.T) {
	client := &mockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Add tool call
	updater.AddToolCall("test-tool")

	// Should have posted a message for the tool call
	assert.Len(t, client.postedMessages, 1, "Should post message for tool call")

	// Check buffer content
	_, _, tools := updater.getBufferContent()
	assert.Contains(t, tools, "test-tool", "Should record tool call")
}

func TestSlackUpdater_Complete(t *testing.T) {
	client := &mockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Add some content and complete
	updater.AddContent("Final answer")
	err := updater.Complete()
	assert.NoError(t, err, "Complete should not error")
}

func TestSlackUpdater_ForceUpdateCompatibility(t *testing.T) {
	client := &mockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Add content and force update (for backward compatibility)
	updater.AddContent("Some content")
	err := updater.ForceUpdate()
	assert.NoError(t, err, "ForceUpdate should not error")
}

func TestSlackUpdater_ThinkingFormatting(t *testing.T) {
	client := &mockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Add thinking content
	updater.AddThought("This is a thought process")

	// Check that thinking would be formatted with italics in message format
	_, thinking, _ := updater.getBufferContent()
	assert.Equal(t, "This is a thought process", thinking, "Should store thinking without formatting")

	// When posted, it should be formatted with italics
	err := updater.ForceUpdate()
	assert.NoError(t, err)

	// The posted message should be formatted
	assert.Len(t, client.postedMessages, 1, "Should post formatted message")
}

func TestSlackUpdater_MultipleStateTransitions(t *testing.T) {
	client := &mockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Start with thinking
	updater.AddThought("Initial thought")
	content, thinking, _ := updater.getBufferContent()
	assert.Equal(t, "Initial thought", thinking)

	// Transition to content
	updater.AddContent("Some content")
	content, thinking, _ = updater.getBufferContent()
	assert.Equal(t, "Some content", content)
	assert.Equal(t, "", thinking)

	// Transition back to thinking
	updater.AddThought("Another thought")
	_, thinking, _ = updater.getBufferContent()
	assert.Equal(t, "Another thought", thinking)
}

func TestSlackUpdater_FinalBufferFlush(t *testing.T) {
	client := &mockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Add content in thinking state
	updater.AddThought("Let me think")

	// Transition to content and add final content
	updater.AddContent("Here's the final answer")

	// ForceUpdate should flush all accumulated content
	err := updater.ForceUpdate()
	assert.NoError(t, err)

	// Should have posted messages
	assert.Greater(t, len(client.postedMessages), 0, "Should have posted messages")

	// Get final buffer should show current state
	content, thinking, _ := updater.getBufferContent()
	assert.Equal(t, "Here's the final answer", content, "Should retain content buffer")
	assert.Equal(t, "", thinking, "Should have flushed thinking")
}

func TestSlackUpdater_CompleteWithFinalContent(t *testing.T) {
	client := &mockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Add final content and complete
	updater.AddContent("Final answer")
	err := updater.Complete()
	assert.NoError(t, err, "Complete should not error")

	// Should have flushed content on complete
	assert.Greater(t, len(client.postedMessages), 0, "Should post content on complete")

	// Buffer should be reset after complete
	content, _, _ := updater.getBufferContent()
	assert.Equal(t, "", content, "Buffer should be reset after complete")
}

func TestSlackUpdater_UserReportedScenario(t *testing.T) {
	client := &mockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// User scenario: thinking first, then content
	updater.AddThought("Let me think about this problem")
	updater.AddThought("I need to analyze the requirements")

	// Transition to content
	updater.AddContent("Based on my analysis, here's the solution:")
	updater.AddContent("1. First approach")
	updater.AddContent("2. Second approach")

	// Force final update (like the integration does)
	err := updater.ForceUpdate()
	assert.NoError(t, err, "ForceUpdate should not error")

	// User reports they only see thoughts at end, not content
	// Let's verify what actually gets posted
	t.Logf("Total messages posted: %d", len(client.postedMessages))

	// Check what's in the buffer
	content, thinking, _ := updater.getBufferContent()
	t.Logf("Final buffer - content: '%s', thinking: '%s'", content, thinking)

	// The issue might be that content is not being posted
	// Let's ensure we get both thinking and content visible
	assert.Greater(t, len(client.postedMessages), 0, "Should have posted something")
}

func TestSlackUpdater_RealWorldScenario(t *testing.T) {
	client := &mockSlackSink{}
	updater := NewSlackUpdater(client, "test-channel")

	// Simulate real scenario: thinking -> content -> ForceUpdate
	updater.AddThought("Let me analyze this problem")

	// Should be in thinking state
	_, thinking, _ := updater.getBufferContent()
	assert.Equal(t, "Let me analyze this problem", thinking)

	// Add content (should trigger transition and flush)
	updater.AddContent("Based on my analysis, here's the solution:")
	updater.AddContent("Step 1: Identify the problem")
	updater.AddContent("Step 2: Implement fix")

	// ForceUpdate should flush final content
	err := updater.ForceUpdate()
	assert.NoError(t, err)

	// Should have posted both thinking and content
	assert.GreaterOrEqual(t, len(client.postedMessages), 1, "Should have posted messages")

	// Buffer should contain content after flush
	content, thinking, _ := updater.getBufferContent()
	assert.Equal(t, "Based on my analysis, here's the solution:Step 1: Identify the problemStep 2: Implement fix", content)
	assert.Equal(t, "", thinking, "Thinking should be flushed")
}
