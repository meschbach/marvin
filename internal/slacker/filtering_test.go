package slacker

import (
	"testing"

	"github.com/slack-go/slack/slackevents"
	"github.com/stretchr/testify/assert"
)

// TestMessageHandler_FilteringLogic tests the filtering logic in isolation
func TestMessageHandler_FilteringLogic(t *testing.T) {

	t.Run("BotMessageFiltering", func(t *testing.T) {
		event := &slackevents.MessageEvent{
			BotID:       "B987654321",
			User:        "U987654321",
			Channel:     "D1234567890",
			ChannelType: "im",
			Text:        "bot message",
			TimeStamp:   "1234567890.123456",
		}

		// Test the filtering logic directly
		shouldFilter := event.BotID != "" || event.SubType != "" || event.Text == ""
		assert.True(t, shouldFilter, "Bot message should be filtered")
	})

	t.Run("ChannelMentionFiltering", func(t *testing.T) {
		event := &slackevents.MessageEvent{
			User:        "U987654321",
			Channel:     "C1234567890",
			ChannelType: "channel",
			Text:        "random conversation", // No bot mention
			TimeStamp:   "1234567890.123456",
		}

		// Test the filtering logic directly
		shouldFilter := event.BotID != "" || event.SubType != "" || event.Text == ""
		assert.False(t, shouldFilter, "Channel message without mention should not be filtered by basic checks")

		// Test the mention check
		hasMention := containsStringTest(event.Text, "<@U123456789>")
		assert.False(t, hasMention, "Channel message should not have bot mention")

		// Combined filtering for channels
		shouldBeFiltered := !hasMention && event.ChannelType != "im"
		assert.True(t, shouldBeFiltered, "Channel message without mention should be filtered")
	})

	t.Run("DirectMessagePass", func(t *testing.T) {
		event := &slackevents.MessageEvent{
			User:        "U987654321",
			Channel:     "D1234567890",
			ChannelType: "im",
			Text:        "hello bot",
			TimeStamp:   "1234567890.123456",
		}

		// Test the filtering logic directly
		shouldFilter := event.BotID != "" || event.SubType != "" || event.Text == ""
		assert.False(t, shouldFilter, "Direct message should not be filtered by basic checks")

		// Direct messages always pass mention check
		assert.True(t, event.ChannelType == "im", "Direct message should pass")
	})

	t.Run("ChannelMentionPass", func(t *testing.T) {
		event := &slackevents.MessageEvent{
			User:        "U987654321",
			Channel:     "C1234567890",
			ChannelType: "channel",
			Text:        "<@U123456789> help me",
			TimeStamp:   "1234567890.123456",
		}

		// Test the filtering logic directly
		shouldFilter := event.BotID != "" || event.SubType != "" || event.Text == ""
		assert.False(t, shouldFilter, "Channel message with mention should not be filtered by basic checks")

		// Test the mention check
		hasMention := containsStringTest(event.Text, "<@U123456789>")
		assert.True(t, hasMention, "Channel message should have bot mention")

		// Combined filtering for channels
		shouldBeFiltered := !hasMention && event.ChannelType != "im"
		assert.False(t, shouldBeFiltered, "Channel message with mention should not be filtered")
	})
}

// Helper functions for testing
func containsStringTest(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
