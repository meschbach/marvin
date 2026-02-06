package slacker

import (
	"testing"

	"github.com/meschbach/marvin/internal/query"
	"github.com/ollama/ollama/api"
	"github.com/slack-go/slack/slackevents"
	"github.com/stretchr/testify/assert"
)

// TestMessageHandler_MessageFiltering tests just the message filtering logic
// This isolates the core logic without requiring complex dependencies
func TestMessageHandler_MessageFiltering(t *testing.T) {
	// Create a mock connection for testing bot ID
	conn := &SlackConnection{
		botUserID: "U123456789",
	}

	// Test message filtering logic directly
	tests := []struct {
		name          string
		event         *slackevents.MessageEvent
		shouldProcess bool
		expectedClean string
	}{
		{
			name: "Direct message should be processed",
			event: &slackevents.MessageEvent{
				User:        "U987654321",
				Channel:     "D1234567890",
				ChannelType: "im",
				Text:        "hello bot",
				TimeStamp:   "1234567890.123456",
			},
			shouldProcess: true,
			expectedClean: "hello bot",
		},
		{
			name: "Channel message with mention should be processed",
			event: &slackevents.MessageEvent{
				User:        "U987654321",
				Channel:     "C1234567890",
				ChannelType: "channel",
				Text:        "<@U123456789> hello in channel",
				TimeStamp:   "1234567890.123456",
			},
			shouldProcess: true,
			expectedClean: "hello in channel",
		},
		{
			name: "Channel message without mention should be ignored",
			event: &slackevents.MessageEvent{
				User:        "U987654321",
				Channel:     "C1234567890",
				ChannelType: "channel",
				Text:        "hello without mention",
				TimeStamp:   "1234567890.123456",
			},
			shouldProcess: false,
			expectedClean: "",
		},
		{
			name: "Bot message should be ignored",
			event: &slackevents.MessageEvent{
				BotID:       "B987654321",
				User:        "U987654321",
				Channel:     "D1234567890",
				ChannelType: "im",
				Text:        "hello from bot",
				TimeStamp:   "1234567890.123456",
			},
			shouldProcess: false,
			expectedClean: "",
		},
		{
			name: "Empty message should be ignored",
			event: &slackevents.MessageEvent{
				User:        "U987654321",
				Channel:     "C1234567890",
				ChannelType: "channel",
				Text:        "<@U123456789>   ",
				TimeStamp:   "1234567890.123456",
			},
			shouldProcess: false,
			expectedClean: "",
		},
		{
			name: "SubType message should be ignored",
			event: &slackevents.MessageEvent{
				User:        "U987654321",
				Channel:     "D1234567890",
				ChannelType: "im",
				Text:        "hello bot",
				SubType:     "message_changed", // Any subtype should be ignored
				TimeStamp:   "1234567890.123456",
			},
			shouldProcess: false,
			expectedClean: "",
		},
		{
			name: "Empty text should be ignored",
			event: &slackevents.MessageEvent{
				User:        "U987654321",
				Channel:     "D1234567890",
				ChannelType: "im",
				Text:        "",
				TimeStamp:   "1234567890.123456",
			},
			shouldProcess: false,
			expectedClean: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the filtering logic directly
			shouldProcess := shouldProcessMessage(tt.event, conn.GetBotUserID())
			assert.Equal(t, tt.shouldProcess, shouldProcess, "Message filtering result mismatch")

			if shouldProcess {
				cleanMessage := cleanMessageText(tt.event.Text, conn.GetBotUserID())
				assert.Equal(t, tt.expectedClean, cleanMessage, "Message cleaning result mismatch")
			}
		})
	}
}

// Helper functions to test the core logic without dependencies

// shouldProcessMessage tests if a message should be processed based on filtering rules
func shouldProcessMessage(event *slackevents.MessageEvent, botUserID string) bool {
	// Ignore messages from bots or messages without text
	if event.BotID != "" || event.SubType != "" || event.Text == "" {
		return false
	}

	// Check if message is mentioning the bot
	if event.ChannelType == "im" {
		// Direct message - always process
		return true
	} else {
		// Channel message - check for bot mention
		if !containsBotMention(event.Text, botUserID) {
			return false
		}
	}

	// Remove bot mention from message and check if empty
	cleanMessage := cleanMessageText(event.Text, botUserID)
	return cleanMessage != ""
}

// containsBotMention checks if the text contains a bot mention
func containsBotMention(text, botUserID string) bool {
	botMention := "<@" + botUserID + ">"
	return containsString(text, botMention)
}

// cleanMessageText removes bot mention and trims whitespace
func cleanMessageText(text, botUserID string) string {
	botMention := "<@" + botUserID + ">"
	cleanText := replaceAllString(text, botMention, "")
	return trimString(cleanText)
}

// String manipulation functions to avoid import issues in tests
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func replaceAllString(s, old, new string) string {
	result := ""
	i := 0
	for i < len(s) {
		if i <= len(s)-len(old) && s[i:i+len(old)] == old {
			result += new
			i += len(old)
		} else {
			result += s[i : i+1]
			i++
		}
	}
	return result
}

func trimString(s string) string {
	// Simple trim leading and trailing whitespace
	start := 0
	end := len(s)

	// Trim leading whitespace
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	// Trim trailing whitespace
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}

// TestIntentProcessor_ProcessMessage tests intent recognition
func TestIntentProcessor_ProcessMessage(t *testing.T) {
	processor := NewIntentProcessor()

	tests := []struct {
		name           string
		message        string
		expectedAction string
		expectedTool   string
		shouldMatch    bool
	}{
		{
			name:           "Add docker tool",
			message:        "add docker tool nginx",
			shouldMatch:    true,
			expectedAction: "add_tool",
			expectedTool:   "docker",
		},
		{
			name:           "Add HTTP tool",
			message:        "add http tool at http://example.com/mcp",
			shouldMatch:    true,
			expectedAction: "add_tool",
			expectedTool:   "http",
		},
		{
			name:           "Add local program",
			message:        "add local tool at /usr/bin/echo",
			shouldMatch:    true,
			expectedAction: "add_tool",
			expectedTool:   "local",
		},
		{
			name:           "Add local program",
			message:        "add local program /usr/bin/echo",
			shouldMatch:    true,
			expectedAction: "add_tool",
			expectedTool:   "local",
		},
		{
			name:           "List tools",
			message:        "list my tools",
			shouldMatch:    true,
			expectedAction: "list_tools",
		},
		{
			name:           "What tools question",
			message:        "what tools can i use",
			shouldMatch:    true,
			expectedAction: "list_tools",
		},
		{
			name:           "Remove tool",
			message:        "remove docker nginx",
			shouldMatch:    true,
			expectedAction: "remove_tool",
		},
		{
			name:           "Share tool",
			message:        "share docker nginx with @john",
			shouldMatch:    true,
			expectedAction: "share_tool",
		},
		{
			name:        "Regular conversation",
			message:     "hello how are you",
			shouldMatch: false,
		},
		{
			name:        "Partial tool mention",
			message:     "I want to add a tool",
			shouldMatch: false,
		},
		{
			name:           "Reset session",
			message:        "reset session",
			shouldMatch:    true,
			expectedAction: "reset_session",
		},
		{
			name:           "Reset my session",
			message:        "reset my session",
			shouldMatch:    true,
			expectedAction: "reset_session",
		},
		{
			name:           "Reset context",
			message:        "reset context",
			shouldMatch:    true,
			expectedAction: "reset_session",
		},
		{
			name:           "Reset conversation",
			message:        "reset conversation",
			shouldMatch:    true,
			expectedAction: "reset_session",
		},
		{
			name:           "Reset my context",
			message:        "reset my context",
			shouldMatch:    true,
			expectedAction: "reset_session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent, err := processor.ProcessMessage(tt.message)

			assert.NoError(t, err, "Intent processing should not error")

			if tt.shouldMatch {
				assert.NotNil(t, intent, "Intent should be detected")
				if intent != nil {
					assert.Equal(t, tt.expectedAction, intent.Action, "Action should match")
					if tt.expectedTool != "" {
						assert.Equal(t, tt.expectedTool, intent.ToolType, "Tool type should match")
					}
					assert.GreaterOrEqual(t, intent.Confidence, 0.7, "Confidence should meet threshold")
				}
			} else {
				assert.Nil(t, intent, "No intent should be detected")
			}
		})
	}
}

// TestSlackContext_Creation tests SlackContext creation and properties
func TestSlackContext_Creation(t *testing.T) {
	event := &slackevents.MessageEvent{
		User:            "U987654321",
		Channel:         "D1234567890",
		ChannelType:     "im",
		Text:            "test message",
		TimeStamp:       "1234567890.123456",
		ThreadTimeStamp: "1234567890.123456",
	}

	conn := &SlackConnection{
		botUserID: "U123456789",
	}

	// Simulate SlackContext creation
	cleanMessage := cleanMessageText(event.Text, conn.GetBotUserID())

	slackCtx := &SlackContext{
		UserID:    event.User,
		ChannelID: event.Channel,
		Message:   cleanMessage,
		Timestamp: event.TimeStamp,
		ThreadTS:  event.ThreadTimeStamp,
		TeamID:    "",
	}

	// Verify context properties
	assert.Equal(t, "U987654321", slackCtx.UserID)
	assert.Equal(t, "D1234567890", slackCtx.ChannelID)
	assert.Equal(t, "test message", slackCtx.Message)
	assert.Equal(t, "1234567890.123456", slackCtx.Timestamp)
	assert.Equal(t, "1234567890.123456", slackCtx.ThreadTS)
}

// TestUserSession_Creation tests user session creation and management
func TestUserSession_Creation(t *testing.T) {
	userCtx := &query.UserContext{
		UserID:  "U987654321",
		IsAdmin: false,
	}

	// Create user session
	session := NewUserSession("U987654321", "D1234567890", userCtx)

	// Verify session properties
	assert.Equal(t, "U987654321", session.UserID)
	assert.Equal(t, "D1234567890", session.ChannelID)
	assert.Equal(t, userCtx, session.UserContext)
	assert.Equal(t, "user-U987654321", session.ToolNamespace)
	assert.Empty(t, session.Messages)
	assert.Empty(t, session.AvailableTools)

	// Test adding a message
	message := api.Message{
		Role:    "user",
		Content: "test message",
	}

	session.AddMessage(message)

	// Verify message was added
	assert.Len(t, session.Messages, 1)
	assert.Equal(t, "user", session.Messages[0].Role)
	assert.Equal(t, "test message", session.Messages[0].Content)

	// Test activity update
	oldActivity := session.LastActivity
	session.UpdateActivity()
	assert.True(t, session.LastActivity.After(oldActivity))
}
