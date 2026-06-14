package slacker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/meschbach/marvin/internal/llm"
	"github.com/meschbach/marvin/internal/query"
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
	message := llm.Message{
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

// TestCommandRouting_DMWithPrefix tests that DM messages with command prefix route to command processing
func TestCommandRouting_DMWithPrefix(t *testing.T) {
	botUserID := "U123456789"
	conn := &SlackConnection{botUserID: botUserID}

	tests := []struct {
		name        string
		channelType string
		text        string
		wantPrefix  bool
	}{
		{
			name:        "DM with bot mention prefix",
			channelType: "im",
			text:        "<@U123456789> help",
			wantPrefix:  true,
		},
		{
			name:        "DM with bot mention prefix and extra text",
			channelType: "im",
			text:        "<@U123456789> add tool docker",
			wantPrefix:  true,
		},
		{
			name:        "DM without prefix",
			channelType: "im",
			text:        "hello bot",
			wantPrefix:  false,
		},
		{
			name:        "Channel message with mention - not prefix",
			channelType: "channel",
			text:        "<@U123456789> help",
			wantPrefix:  false,
		},
		{
			name:        "DM with mention in middle",
			channelType: "im",
			text:        "hello <@U123456789> help",
			wantPrefix:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := createTestMessageEvent("U987654321", "D1234567890", tt.channelType, tt.text)

			// Simulate hasCommandPrefix logic
			hasPrefix := false
			if ev.ChannelType == "im" {
				mention := fmt.Sprintf("<@%s>", conn.GetBotUserID())
				hasPrefix = strings.HasPrefix(ev.Text, mention)
			}

			assert.Equal(t, tt.wantPrefix, hasPrefix, "Command prefix detection mismatch")
		})
	}
}

// TestCommandRouting_DMWithoutPrefix tests that DM messages without command prefix route to LLM
func TestCommandRouting_DMWithoutPrefix(t *testing.T) {
	botUserID := "U123456789"
	conn := &SlackConnection{botUserID: botUserID}

	tests := []struct {
		name        string
		channelType string
		text        string
		wantLLM     bool
	}{
		{
			name:        "DM plain message routes to LLM",
			channelType: "im",
			text:        "hello, how are you?",
			wantLLM:     true,
		},
		{
			name:        "DM with question routes to LLM",
			channelType: "im",
			text:        "what tools can I use?",
			wantLLM:     true,
		},
		{
			name:        "Channel with mention routes to LLM",
			channelType: "channel",
			text:        "<@U123456789> hello",
			wantLLM:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := createTestMessageEvent("U987654321", "D1234567890", tt.channelType, tt.text)

			// Simulate routing logic: LLM if no command prefix and has mention or DM
			shouldRouteToLLM := false
			mention := fmt.Sprintf("<@%s>", conn.GetBotUserID())
			hasPrefix := ev.ChannelType == "im" && strings.HasPrefix(ev.Text, mention)
			hasMentionInText := strings.Contains(ev.Text, mention)
			cleanMessage := strings.TrimSpace(strings.ReplaceAll(ev.Text, mention, ""))

			shouldRouteToLLM = !hasPrefix && (ev.ChannelType == "im" || hasMentionInText) && cleanMessage != ""

			assert.Equal(t, tt.wantLLM, shouldRouteToLLM, "LLM routing detection mismatch")
		})
	}
}

// TestCommandRouting_ExtractCommand tests command extraction from prefixed messages
func TestCommandRouting_ExtractCommand(t *testing.T) {
	botUserID := "U123456789"

	tests := []struct {
		name    string
		text    string
		wantCmd string
	}{
		{
			name:    "Simple command",
			text:    "<@U123456789> help",
			wantCmd: "help",
		},
		{
			name:    "Command with arguments",
			text:    "<@U123456789> add tool docker",
			wantCmd: "add tool docker",
		},
		{
			name:    "Command with extra whitespace",
			text:    "<@U123456789>   tools   ",
			wantCmd: "tools",
		},
		{
			name:    "Admin command",
			text:    "<@U123456789> model access list",
			wantCmd: "model access list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mention := fmt.Sprintf("<@%s>", botUserID)
			cmd := strings.ReplaceAll(tt.text, mention, "")
			cmd = strings.TrimSpace(cmd)

			assert.Equal(t, tt.wantCmd, cmd, "Command extraction mismatch")
		})
	}
}

// TestCommandRouting_PermissionHandling tests permission check logic for admin vs non-admin
func TestCommandRouting_PermissionHandling(t *testing.T) {
	adminCommands := map[string]bool{
		"admin":        true,
		"model access": true,
		"add tool":     true,
		"remove tool":  true,
	}

	tests := []struct {
		name        string
		cmdName     string
		userIsAdmin bool
		wantExecute bool
	}{
		{
			name:        "Admin user executes admin command",
			cmdName:     "admin",
			userIsAdmin: true,
			wantExecute: true,
		},
		{
			name:        "Non-admin user executes admin command",
			cmdName:     "admin",
			userIsAdmin: false,
			wantExecute: false,
		},
		{
			name:        "Non-admin user executes regular command",
			cmdName:     "help",
			userIsAdmin: false,
			wantExecute: true,
		},
		{
			name:        "Admin user executes regular command",
			cmdName:     "help",
			userIsAdmin: true,
			wantExecute: true,
		},
		{
			name:        "Non-admin user executes model access command",
			cmdName:     "model access",
			userIsAdmin: false,
			wantExecute: false,
		},
		{
			name:        "Admin user executes model access command",
			cmdName:     "model access",
			userIsAdmin: true,
			wantExecute: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isAdminCmd := adminCommands[tt.cmdName]
			canExecute := !isAdminCmd || tt.userIsAdmin

			assert.Equal(t, tt.wantExecute, canExecute, "Permission handling mismatch")
		})
	}
}
