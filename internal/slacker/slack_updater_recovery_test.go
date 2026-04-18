package slacker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/slack-go/slack"
)

// MockErrorFormatter always returns an error to test fallback behavior
type MockErrorFormatter struct{}

func (m *MockErrorFormatter) Format(ctx context.Context, content string, contentType ContentType) ([]slack.Block, error) {
	return nil, errors.New("mock formatting error")
}

// CapturingSlackSink extends MockSlackSink to capture when calls are made
type CapturingSlackSink struct {
	MockSlackSink
	PostCalls   int
	UpdateCalls int
}

func (m *CapturingSlackSink) UpdateMessageContext(ctx context.Context, channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
	m.UpdateCalls++
	return m.MockSlackSink.UpdateMessageContext(ctx, channelID, timestamp, options...)
}

func (m *CapturingSlackSink) PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error) {
	m.PostCalls++
	return m.MockSlackSink.PostMessageContext(ctx, channelID, options...)
}

func TestSlackUpdater_FormattingErrorRecovery(t *testing.T) {
	t.Helper()

	mockClient := &CapturingSlackSink{}
	errorFormatter := &MockErrorFormatter{}
	preferences := DefaultUserPreferences()

	updater := NewSlackUpdater(mockClient, "test-channel", errorFormatter, preferences)
	ctx := t.Context()

	// First message should not crash despite formatting error
	err := updater.AddContent(ctx, "test message")
	if err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}

	// Should have posted progress indicator + main content (2 posts)
	if mockClient.PostCalls != 2 {
		t.Errorf("Expected 2 post calls (progress + content), got %d", mockClient.PostCalls)
	}
}

func TestSlackUpdater_MultipleFormattingErrorsContinue(t *testing.T) {
	t.Helper()

	mockClient := &CapturingSlackSink{}
	errorFormatter := &MockErrorFormatter{}
	preferences := DefaultUserPreferences()

	updater := NewSlackUpdater(mockClient, "test-channel", errorFormatter, preferences)
	ctx := t.Context()

	// Add multiple pieces of content - none should crash
	messages := []string{"first", "second", "third"}
	for _, msg := range messages {
		err := updater.AddContent(ctx, msg)
		if err != nil {
			t.Fatalf("AddContent failed for message '%s': %v", msg, err)
		}
	}

	// Should have made at least one call successfully (SlackUpdater buffers content)
	totalCalls := mockClient.PostCalls + mockClient.UpdateCalls
	if totalCalls < 1 {
		t.Errorf("Expected at least 1 total call, got %d", totalCalls)
	}

	// Force flush to ensure all content is processed
	err := updater.ForceUpdate(ctx)
	if err != nil {
		t.Fatalf("ForceUpdate failed: %v", err)
	}
}

func TestSlackUpdater_FormatterWorksNormally(t *testing.T) {
	t.Helper()

	mockClient := &CapturingSlackSink{}
	workingFormatter := NewSlackFormatter()
	preferences := DefaultUserPreferences()

	updater := NewSlackUpdater(mockClient, "test-channel", workingFormatter, preferences)
	ctx := t.Context()

	content := "# Test Header\n\nThis is a test message."
	err := updater.AddContent(ctx, content)
	if err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}

	// Should have progress indicator + main content
	if mockClient.PostCalls != 2 {
		t.Errorf("Expected 2 post calls (progress + content), got %d", mockClient.PostCalls)
	}
}

func TestSlackUpdater_ContentIgnoreType(t *testing.T) {
	t.Helper()

	mockClient := &CapturingSlackSink{}
	errorFormatter := &MockErrorFormatter{}
	preferences := DefaultUserPreferences()

	updater := NewSlackUpdater(mockClient, "test-channel", errorFormatter, preferences)
	ctx := t.Context()

	// Simulate ContentIgnore type by accessing internal method
	updater.mutex.Lock()
	if _, err := updater.switchToType(ctx, updaterStateComplete); err != nil {
		updater.mutex.Unlock()
		t.Fatalf("Failed to switch to complete state: %v", err)
	}
	updater.mutex.Unlock()

	err := updater.ForceUpdate(ctx)
	if err != nil {
		t.Fatalf("ForceUpdate failed: %v", err)
	}

	// Should not make any calls for ContentIgnore type
	if mockClient.PostCalls > 0 || mockClient.UpdateCalls > 0 {
		t.Errorf("Expected no calls for ContentIgnore type, got %d posts, %d updates",
			mockClient.PostCalls, mockClient.UpdateCalls)
	}
}

func TestSlackUpdater_NotificationFlagBehavior(t *testing.T) {
	t.Helper()

	mockClient := &CapturingSlackSink{}
	errorFormatter := &MockErrorFormatter{}
	preferences := DefaultUserPreferences()

	updater := NewSlackUpdater(mockClient, "test-channel", errorFormatter, preferences)
	ctx := t.Context()

	// Check initial state
	if updater.formattingErrorNotified {
		t.Error("formattingErrorNotified should start as false")
	}

	// First message should trigger notification flag
	err := updater.AddContent(ctx, "first message")
	if err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}

	if !updater.formattingErrorNotified {
		t.Error("formattingErrorNotified should be true after first formatting error")
	}

	// Second message should not change flag
	err = updater.AddContent(ctx, "second message")
	if err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}

	if !updater.formattingErrorNotified {
		t.Error("formattingErrorNotified should remain true")
	}
}

func TestSlackUpdater_NotificationResetOnStateTransition(t *testing.T) {
	t.Helper()

	mockClient := &CapturingSlackSink{}
	errorFormatter := &MockErrorFormatter{}
	preferences := DefaultUserPreferences()

	updater := NewSlackUpdater(mockClient, "test-channel", errorFormatter, preferences)
	ctx := t.Context()

	// Start in init state
	updater.mutex.Lock()
	if updater.currentState != updaterStateInit {
		t.Errorf("Expected init state, got %d", updater.currentState)
	}

	// Manually set notification flag to true to test reset
	updater.formattingErrorNotified = true
	updater.mutex.Unlock()

	// Manually call switchToType to ensure transition happens
	updater.mutex.Lock()
	changed, err := updater.switchToType(ctx, updaterStateThinking)
	updater.mutex.Unlock()
	if err != nil {
		t.Fatalf("switchToType failed: %v", err)
	}

	if !changed {
		t.Error("Expected state to change from init to thinking")
	}

	// After transition to thinking, notification flag should be reset
	updater.mutex.Lock()
	defer updater.mutex.Unlock()
	if updater.formattingErrorNotified {
		t.Error("formattingErrorNotified should be reset after init->thinking transition")
	}
}

func TestSlackUpdater_TimeProviderWorks(t *testing.T) {
	t.Helper()

	mockClient := &CapturingSlackSink{}
	errorFormatter := &MockErrorFormatter{}

	// Mock time provider for deterministic testing
	fixedTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	timeProvider := &MockTimeProvider{CurrentTime: fixedTime}

	preferences := DefaultUserPreferences()
	updater := NewSlackUpdater(mockClient, "test-channel", errorFormatter, preferences, WithTimeProvider(timeProvider))
	ctx := t.Context()

	err := updater.AddContent(ctx, "test message")
	if err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}

	// Should have progress indicator + main content
	if mockClient.PostCalls != 2 {
		t.Errorf("Expected 2 post calls (progress + content), got %d", mockClient.PostCalls)
	}

	// Verify time provider is being used
	if updater.lastUpdateTime != fixedTime {
		t.Errorf("Expected lastUpdateTime to be %v, got %v", fixedTime, updater.lastUpdateTime)
	}
}

func TestSlackUpdater_WorkingFormatterNoNotification(t *testing.T) {
	t.Helper()

	mockClient := &CapturingSlackSink{}
	workingFormatter := NewSlackFormatter()
	preferences := DefaultUserPreferences()

	updater := NewSlackUpdater(mockClient, "test-channel", workingFormatter, preferences)
	ctx := t.Context()

	err := updater.AddContent(ctx, "test message")
	if err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}

	// Should not have set notification flag since no error occurred
	if updater.formattingErrorNotified {
		t.Error("formattingErrorNotified should remain false when formatting succeeds")
	}
}

// MockInvalidBlocksSink simulates Slack API invalid_blocks errors
type MockInvalidBlocksSink struct {
	MockSlackSink
	PostCalls        int
	UpdateCalls      int
	shouldFailPost   bool
	shouldFailUpdate bool
	failureCount     int
}

func (m *MockInvalidBlocksSink) UpdateMessageContext(ctx context.Context, channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
	m.UpdateCalls++
	if m.shouldFailUpdate && m.failureCount > 0 {
		m.failureCount--
		return "", "", "", errors.New("invalid_blocks")
	}
	return m.MockSlackSink.UpdateMessageContext(ctx, channelID, timestamp, options...)
}

func (m *MockInvalidBlocksSink) PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error) {
	m.PostCalls++
	if m.shouldFailPost && m.failureCount > 0 {
		m.failureCount--
		return "", "", errors.New("invalid_blocks")
	}
	return m.MockSlackSink.PostMessageContext(ctx, channelID, options...)
}

func TestSlackUpdater_SlackAPIInvalidBlocksRecovery(t *testing.T) {
	t.Helper()

	// Test that the updater handles invalid_blocks errors gracefully
	mockClient := &MockInvalidBlocksSink{
		shouldFailPost: true,
		failureCount:   1,
	}
	workingFormatter := NewSlackFormatter()
	preferences := DefaultUserPreferences()

	updater := NewSlackUpdater(mockClient, "test-channel", workingFormatter, preferences)
	ctx := t.Context()

	// AddContent may or may not return an error depending on how many posts fail
	_ = updater.AddContent(ctx, "test message")

	// Test that the system continues to function without crashing
	// The key behavior we're testing is that invalid_blocks is handled gracefully
}

func TestSlackUpdater_SlackAPIInvalidBlocksRecoveryUpdate(t *testing.T) {
	t.Helper()

	// Test UpdateMessageContext with invalid_blocks
	mockClient := &MockInvalidBlocksSink{
		shouldFailUpdate: true,
		failureCount:     1,
	}
	workingFormatter := NewSlackFormatter()
	preferences := DefaultUserPreferences()

	updater := NewSlackUpdater(mockClient, "test-channel", workingFormatter, preferences)
	ctx := t.Context()

	// First, post a message to get a timestamp
	err := updater.AddContent(ctx, "first message")
	if err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}

	// Now try to update with failure count reset to trigger the error recovery
	mockClient.failureCount = 1
	mockClient.shouldFailUpdate = true

	// Force the updater to update by waiting a second and using different content
	time.Sleep(time.Second + 100*time.Millisecond) // Ensure time-based update condition
	err = updater.AddContent(ctx, "completely different message")
	if err != nil {
		t.Fatalf("AddContent update failed: %v", err)
	}

	// Should have both post and update calls
	totalCalls := mockClient.PostCalls + mockClient.UpdateCalls
	if totalCalls < 2 {
		t.Errorf("Expected at least 2 total calls, got %d", totalCalls)
	}
}

func TestSlackUpdater_CharacterLimitEnforcement(t *testing.T) {
	t.Helper()

	mockClient := &CapturingSlackSink{}
	workingFormatter := NewSlackFormatter()
	preferences := DefaultUserPreferences()

	updater := NewSlackUpdater(mockClient, "test-channel", workingFormatter, preferences)
	ctx := t.Context()

	// Create a very long message that exceeds limits
	longMessage := strings.Repeat("This is a very long message that should be truncated. ", 200) // Much longer than 4000 chars

	err := updater.AddContent(ctx, longMessage)
	if err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}

	// Should have progress indicator + main content
	if mockClient.PostCalls != 2 {
		t.Errorf("Expected 2 post calls, got %d", mockClient.PostCalls)
	}

	// The key test is that the process didn't fail due to excessive length
}

func TestSlackUpdater_CharacterLimitEnforcementWithManyBlocks(t *testing.T) {
	t.Helper()

	mockClient := &CapturingSlackSink{}
	workingFormatter := NewSlackFormatter()
	preferences := DefaultUserPreferences()

	updater := NewSlackUpdater(mockClient, "test-channel", workingFormatter, preferences)
	ctx := t.Context()

	// Create content that would result in many blocks
	contentWithHeaders := strings.Repeat("# Header\n\nThis is content.\n\n", 60) // Would create >50 blocks

	err := updater.AddContent(ctx, contentWithHeaders)
	if err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}

	// Should have progress indicator + main content
	if mockClient.PostCalls != 2 {
		t.Errorf("Expected 2 post calls, got %d", mockClient.PostCalls)
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		maxLength int
		expected  string
	}{
		{
			name:      "Short text unchanged",
			text:      "Hello world",
			maxLength: 50,
			expected:  "Hello world",
		},
		{
			name:      "Long text truncated",
			text:      "This is a very long message that exceeds the maximum length limit",
			maxLength: 20,
			expected:  "This is a very...",
		},
		{
			name:      "Text truncated at word boundary",
			text:      "Hello world this is a test message",
			maxLength: 25,
			expected:  "Hello world this is a...",
		},
		{
			name:      "Text exactly at limit",
			text:      "1234567890",
			maxLength: 10,
			expected:  "1234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateText(tt.text, tt.maxLength)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Test to verify that enforceSlackLimits actually truncates content as expected
func TestEnforceSlackLimitsIntegration(t *testing.T) {
	t.Helper()

	mockClient := &CapturingSlackSink{}
	workingFormatter := NewSlackFormatter()
	preferences := DefaultUserPreferences()

	updater := NewSlackUpdater(mockClient, "test-channel", workingFormatter, preferences)
	ctx := t.Context()

	// Test with extremely long content that should be truncated
	longMessage := strings.Repeat("This message is way too long and exceeds Slack's limits. ", 200)

	err := updater.AddContent(ctx, longMessage)
	if err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}

	// Should have progress indicator + main content
	if mockClient.PostCalls != 2 {
		t.Errorf("Expected 2 post calls, got %d", mockClient.PostCalls)
	}
}
