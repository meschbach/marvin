package slacker

import (
	"strings"
	"testing"

	"github.com/meschbach/marvin/internal/query"
	"github.com/ollama/ollama/api"
	"github.com/slack-go/slack/slackevents"
	"github.com/stretchr/testify/assert"
)

// createTestSlackConnection creates a test SlackConnection with consistent bot ID
func createTestSlackConnection() *SlackConnection {
	return &SlackConnection{botUserID: "U123456789"}
}

// createTestMessageEvent creates a test message event with common fields
func createTestMessageEvent(user, channel, channelType, text string) *slackevents.MessageEvent {
	return &slackevents.MessageEvent{
		User:        user,
		Channel:     channel,
		ChannelType: channelType,
		Text:        text,
		TimeStamp:   "1234567890.123456",
	}
}

// TestMessageHandler_MessageFiltering tests just the message filtering logic
// This isolates the core logic without requiring complex dependencies
func TestMessageHandler_MessageFiltering(t *testing.T) {
	conn := createTestSlackConnection()

	// Test message filtering logic directly
	tests := []struct {
		name        string
		event       *slackevents.MessageEvent
		wantProcess bool
		wantClean   string
	}{
		{"Direct message should be processed", createTestMessageEvent("U987654321", "D1234567890", "im", "hello bot"), true, "hello bot"},
		{"Channel message with mention should be processed", createTestMessageEvent("U987654321", "C1234567890", "channel", "<@U123456789> hello in channel"), true, "hello in channel"},
		{"Channel message without mention should be ignored", createTestMessageEvent("U987654321", "C1234567890", "channel", "hello without mention"), false, ""},
		{"Bot message should be ignored", func() *slackevents.MessageEvent {
			e := createTestMessageEvent("U987654321", "D1234567890", "im", "hello from bot")
			e.BotID = "B987654321"
			return e
		}(), false, ""},
		{"Empty message should be ignored", createTestMessageEvent("U987654321", "C1234567890", "channel", "<@U123456789>   "), false, ""},
		{"SubType message should be ignored", func() *slackevents.MessageEvent {
			e := createTestMessageEvent("U987654321", "D1234567890", "im", "hello bot")
			e.SubType = "message_changed"
			return e
		}(), false, ""},
		{"Empty text should be ignored", createTestMessageEvent("U987654321", "D1234567890", "im", ""), false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			botUserID := conn.GetBotUserID()

			// Test filtering logic inline
			shouldProcess := tt.event.BotID == "" && tt.event.SubType == "" && tt.event.Text != "" &&
				(tt.event.ChannelType == "im" || strings.Contains(tt.event.Text, "<@"+botUserID+">")) &&
				strings.TrimSpace(strings.ReplaceAll(tt.event.Text, "<@"+botUserID+">", "")) != ""

			assert.Equal(t, tt.wantProcess, shouldProcess, "Message filtering result mismatch")

			if shouldProcess {
				cleanMessage := strings.TrimSpace(strings.ReplaceAll(tt.event.Text, "<@"+botUserID+">", ""))
				assert.Equal(t, tt.wantClean, cleanMessage, "Message cleaning result mismatch")
			}
		})
	}
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
	event := createTestMessageEvent("U987654321", "D1234567890", "im", "test message")
	event.ThreadTimeStamp = "1234567890.123456"

	conn := createTestSlackConnection()

	// Simulate SlackContext creation
	botUserID := conn.GetBotUserID()
	cleanMessage := strings.TrimSpace(strings.ReplaceAll(event.Text, "<@"+botUserID+">", ""))

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
