package slacker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	"github.com/meschbach/marvin/internal/slacker/security"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMessageFlow_Integration tests the complete message flow from Slack to Ollama
//
//nolint:funlen
func TestMessageFlow_Integration(t *testing.T) {
	// Create temporary directory for session storage
	tempDir, err := os.MkdirTemp("", "marvin-test-*")
	require.NoError(t, err)
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to cleanup temp directory %s: %v", tempDir, err)
		}
	}()

	// Create security logger
	logger := security.NewSecurityLogger()

	// Create session manager
	sessionManager, err := NewSessionManager(tempDir)
	require.NoError(t, err)

	// Create a simple test configuration with a model for integration tests
	cfg := &config.File{
		Model: "test-model", // Set a model name
	}

	// Create tenant tool set with context
	ctx := t.Context()
	tenantToolSet, err := query.NewTenantToolSet(ctx, cfg)
	require.NoError(t, err)

	// Create formatter
	formatter := NewSlackFormatter()

	// Create help integrator for tests
	mockLLM := &MockLLM{}
	helpAnalyzer := NewHelpAnalyzer(mockLLM, cfg, sessionManager, tenantToolSet, tenantToolSet)
	contextBuilder := NewHelpContextBuilder(sessionManager, cfg, tenantToolSet)
	helpIntegrator := NewHelpIntegrator(helpAnalyzer, contextBuilder)

	// Create query processor
	queryProcessor, err := NewQueryProcessor(tenantToolSet, sessionManager, nil, cfg, logger, formatter, helpIntegrator)
	require.NoError(t, err)

	// Create intent processor
	intentProcessor := NewIntentProcessor()

	// Create approval workflow
	approvalWorkflow := NewApprovalWorkflow([]string{}, logger)

	// Create mock notification sender to avoid actual Slack calls
	notificationSender := NewMockNotificationSender()

	// Create tool manager
	toolManager := NewToolManager(approvalWorkflow, tenantToolSet, logger, notificationSender, sessionManager, helpIntegrator)

	// Create a mock connection (create a real client to avoid panics in GetUserInfo)
	conn := &SlackConnection{
		botUserID: "U123456789",
		client:    slack.New("test-token", slack.OptionAppLevelToken("test-app-token")),
	}

	// Create message handler for integration tests
	messageHandler := NewMessageHandler(
		intentProcessor,
		conn,
		queryProcessor,
		toolManager,
		sessionManager,
		logger,
		cfg,
		tenantToolSet,
	)

	t.Run("DirectMessageFlow", func(t *testing.T) {
		// Step 1: Create a Slack message event
		event := &slackevents.MessageEvent{
			User:        "U987654321",
			Channel:     "D1234567890", // DM channel
			ChannelType: "im",
			Text:        "hello, how are you?",
			TimeStamp:   "1234567890.123456",
		}

		// Step 2: Process the message through the message handler
		err := messageHandler.ProcessMessage(ctx, event)

		// Should not error even if Ollama is not available (it will fail gracefully)
		assert.NoError(t, err, "Message processing should not return error")

		// Step 3: Verify session was created and user message added
		session, exists := sessionManager.GetSession("U987654321", "D1234567890")
		assert.True(t, exists, "Session should be created")
		assert.NotNil(t, session)
		assert.Equal(t, "U987654321", session.UserID)
		assert.Equal(t, "D1234567890", session.ChannelID)

		// Step 4: Verify user message was added to session
		require.Len(t, session.Messages, 1, "User message should be added to session")
		assert.Equal(t, "user", session.Messages[0].Role)
		assert.Equal(t, "hello, how are you?", session.Messages[0].Content)

		// Step 5: Verify session persistence
		sessionFile := filepath.Join(tempDir, "session-U987654321-D1234567890.json")
		assert.FileExists(t, sessionFile, "Session file should be created")
	})

	t.Run("ToolIntentFlow", func(t *testing.T) {
		// Step 1: Create a tool intent message
		event := &slackevents.MessageEvent{
			User:        "U987654321",
			Channel:     "D1234567891",
			ChannelType: "im",
			Text:        "add docker tool nginx",
			TimeStamp:   "1234567890.123456",
		}

		// Step 2: Process the message
		err := messageHandler.ProcessMessage(ctx, event)
		assert.NoError(t, err, "Tool intent processing should not return error")

		// Step 3: Verify session was created but no user message added (tool intent handled differently)
		session, exists := sessionManager.GetSession("U987654321", "D1234567891")
		assert.True(t, exists, "Session should be created for tool intent")
		assert.NotNil(t, session)
		// Tool intent messages are not added to session history
		assert.Empty(t, session.Messages, "Tool intent messages should not be added to session")

		// Additional verification: Check that approval workflow was triggered
		// We can't easily verify this without mocks, but the lack of panic is good
	})

	t.Run("ChannelMentionFlow", func(t *testing.T) {
		// Step 1: Create a channel message with bot mention
		event := &slackevents.MessageEvent{
			User:        "U987654321",
			Channel:     "C1234567890", // Channel
			ChannelType: "channel",
			Text:        "<@U123456789> what's the weather?",
			TimeStamp:   "1234567890.123456",
		}

		// Step 2: Process the message
		err := messageHandler.ProcessMessage(ctx, event)
		assert.NoError(t, err, "Channel message with mention should not return error")

		// Step 3: Verify session was created and message was cleaned
		session, exists := sessionManager.GetSession("U987654321", "C1234567890")
		assert.True(t, exists, "Session should be created for channel message")
		assert.NotNil(t, session)

		// Step 4: Verify message was cleaned (bot mention removed)
		require.Len(t, session.Messages, 1, "User message should be added to session")
		assert.Equal(t, "what's the weather?", session.Messages[0].Content, "Bot mention should be removed")
	})

	t.Run("IgnoredMessageFlow", func(t *testing.T) {
		// Step 1: Create a channel message without bot mention (should be ignored)
		event := &slackevents.MessageEvent{
			User:        "U987654321",
			Channel:     "C1234567891", // Different channel
			ChannelType: "channel",
			Text:        "this should be ignored",
			TimeStamp:   "1234567890.123456",
		}

		// Step 2: Process the message
		err := messageHandler.ProcessMessage(ctx, event)
		assert.NoError(t, err, "Ignored message processing should not return error")

		// Step 3: Verify no session was created (message ignored)
		_, exists := sessionManager.GetSession("U987654321", "C1234567891")
		assert.False(t, exists, "No session should be created for ignored message")
	})
}

// TestEventRouter_Integration tests the flow from Socket Mode event to message handler
//
//nolint:funlen
func TestEventRouter_Integration(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "marvin-test-*")
	require.NoError(t, err)
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to cleanup temp directory %s: %v", tempDir, err)
		}
	}()

	// Setup components
	logger := security.NewSecurityLogger()
	sessionManager, err := NewSessionManager(tempDir)
	require.NoError(t, err)

	cfg := &config.File{
		Model: "test-model",
	}
	ctx := t.Context()
	tenantToolSet, err := query.NewTenantToolSet(ctx, cfg)
	require.NoError(t, err)

	formatter := NewSlackFormatter()

	queryProcessor, err := NewQueryProcessor(tenantToolSet, sessionManager, nil, cfg, logger, formatter, nil)
	require.NoError(t, err)
	intentProcessor := NewIntentProcessor()
	approvalWorkflow := NewApprovalWorkflow([]string{}, logger)

	// Create mock notification sender to avoid nil pointer issues
	notificationSender := NewMockNotificationSender()
	toolManager := NewToolManager(approvalWorkflow, tenantToolSet, logger, notificationSender, sessionManager, nil)

	conn := &SlackConnection{
		botUserID: "U123456789",
		client:    slack.New("test-token", slack.OptionAppLevelToken("test-app-token")),
	}

	messageHandler := NewMessageHandler(
		intentProcessor,
		conn,
		queryProcessor,
		toolManager,
		sessionManager,
		logger,
		cfg, // Use config with help disabled
		tenantToolSet,
	)

	// Critical path test cases that users commonly experience
	testCases := []struct {
		name          string
		event         *slackevents.MessageEvent
		shouldProcess bool
		expectedClean string
		description   string
	}{
		{
			name: "ValidDirectMessage",
			event: &slackevents.MessageEvent{
				User:        "U987654321",
				Channel:     "D1234567890",
				ChannelType: "im",
				Text:        "hello bot",
				TimeStamp:   "1234567890.123456",
			},
			shouldProcess: true,
			expectedClean: "hello bot",
			description:   "Direct message should always be processed",
		},
		{
			name: "ChannelWithBotMention",
			event: &slackevents.MessageEvent{
				User:        "U987654321",
				Channel:     "C1234567890",
				ChannelType: "channel",
				Text:        "<@U123456789> help me",
				TimeStamp:   "1234567890.123456",
			},
			shouldProcess: true,
			expectedClean: "help me",
			description:   "Channel message with bot mention should be processed",
		},
		{
			name: "ChannelWithoutMention",
			event: &slackevents.MessageEvent{
				User:        "U987654322",
				Channel:     "C1234567891",
				ChannelType: "channel",
				Text:        "random conversation",
				TimeStamp:   "1234567890.123456",
			},
			shouldProcess: false,
			expectedClean: "",
			description:   "Channel message without mention should be ignored",
		},
		{
			name: "BotMessage",
			event: &slackevents.MessageEvent{
				BotID:       "B987654321",
				User:        "U987654323",
				Channel:     "D1234567891",
				ChannelType: "im",
				Text:        "bot message",
				TimeStamp:   "1234567890.123456",
			},
			shouldProcess: false,
			expectedClean: "",
			description:   "Bot message should be ignored",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Debug: Print message details
			t.Logf("Testing message: User=%s, Channel=%s, ChannelType=%s, Text=%q, BotID=%s, SubType=%s",
				tc.event.User, tc.event.Channel, tc.event.ChannelType, tc.event.Text, tc.event.BotID, tc.event.SubType)

			// Process the message
			err := messageHandler.ProcessMessage(ctx, tc.event)
			assert.NoError(t, err, "Message processing should not error for: %s", tc.description)

			// Check if session was created
			session, exists := sessionManager.GetSession(tc.event.User, tc.event.Channel)
			t.Logf("Session exists: %v, shouldProcess: %v", exists, tc.shouldProcess)

			if tc.shouldProcess {
				assert.True(t, exists, "Session should be created for: %s", tc.description)
				assert.NotNil(t, session)
				assert.Len(t, session.Messages, 1, "Message should be added to session")
				assert.Equal(t, tc.expectedClean, session.Messages[0].Content, "Message content should match")
			} else {
				assert.False(t, exists, "Session should not be created for ignored message: %s", tc.description)
				// Additional check: Verify that the function returned early (nil) for ignored messages
				// This is tested implicitly by the session check, but we can verify the logic works
			}
		})
	}
}
