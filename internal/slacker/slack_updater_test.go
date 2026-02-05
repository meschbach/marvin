package slacker

import (
	"testing"
	"time"

	"github.com/slack-go/slack"
)

// mockSlackSink implements SlackSink for testing
type mockSlackSink struct {
}

func (m *mockSlackSink) UpdateMessage(channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
	return "channel", "timestamp", "text", nil
}

func (m *mockSlackSink) PostMessage(channelID string, options ...slack.MsgOption) (string, string, error) {
	return "channel", "test-timestamp", nil
}

func TestSlackUpdater_Deadlock(t *testing.T) {
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
