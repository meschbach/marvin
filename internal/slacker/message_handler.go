package slacker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/slack-go/slack/slackevents"
)

// QueryHandler defines the interface for handling queries
type QueryHandler interface {
	HandleQueryWithUpdater(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, updater *SlackUpdater) error
}

// MessageProcessor defines the interface for processing messages
type MessageProcessor interface {
	ProcessMessage(ctx context.Context, ev *slackevents.MessageEvent) error
}

// MessageHandler processes incoming Slack messages and intents
type MessageHandler struct {
	intentProcessor *IntentProcessor
	connection      *SlackConnection
	queryHandler    QueryHandler
	toolManager     *ToolManagerImpl
	sessionManager  *SessionManager
	securityLogger  *sec.SecurityLogger
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(
	intentProcessor *IntentProcessor,
	connection *SlackConnection,
	queryHandler QueryHandler,
	toolManager *ToolManagerImpl,
	sessionManager *SessionManager,
	securityLogger *sec.SecurityLogger,
) *MessageHandler {
	return &MessageHandler{
		intentProcessor: intentProcessor,
		connection:      connection,
		queryHandler:    queryHandler,
		toolManager:     toolManager,
		sessionManager:  sessionManager,
		securityLogger:  securityLogger,
	}
}

// ProcessMessage processes incoming Slack messages
func (mh *MessageHandler) ProcessMessage(ctx context.Context, ev *slackevents.MessageEvent) error {
	// Ignore messages from bots or messages without text
	if ev.BotID != "" || ev.SubType != "" || ev.Text == "" {
		return nil
	}

	// Check if message is mentioning the bot
	if ev.ChannelType == "im" {
		// Direct message - always process
	} else {
		// Channel message - check for bot mention
		if !strings.Contains(ev.Text, fmt.Sprintf("<@%s>", mh.connection.GetBotUserID())) {
			return nil
		}
	}

	// Remove bot mention from message
	cleanMessage := strings.ReplaceAll(ev.Text, fmt.Sprintf("<@%s>", mh.connection.GetBotUserID()), "")
	cleanMessage = strings.TrimSpace(cleanMessage)

	if cleanMessage == "" {
		return nil
	}

	// Create Slack context
	slackCtx := &SlackContext{
		UserID:    ev.User,
		ChannelID: ev.Channel,
		Message:   cleanMessage,
		Timestamp: ev.TimeStamp,
		ThreadTS:  ev.ThreadTimeStamp,
		TeamID:    "", // TODO: Get from user info if needed
	}

	// Get user info
	user, err := mh.connection.GetClient().GetUserInfo(ev.User)
	if err == nil {
		slackCtx.UserName = user.Name
	}

	// Log session event
	mh.securityLogger.LogSessionEvent(ev.User, ev.Channel, "Message received")

	// Create user context
	userCtx := &query.UserContext{
		UserID:      ev.User,
		SlackTeamID: slackCtx.TeamID,
		IsAdmin:     false, // This would need to be determined from tenantToolSet
	}

	// Get or create user session
	session := mh.sessionManager.GetOrCreateSession(ev.User, ev.Channel, userCtx)

	// Check if this is a tool management request
	intent, err := mh.intentProcessor.ProcessMessage(cleanMessage)
	if err != nil {
		return fmt.Errorf("processing intent: %w", err)
	}

	if intent != nil && intent.Confidence >= 0.7 {
		// Handle tool management request
		return mh.toolManager.HandleToolIntent(ctx, slackCtx, session, intent)
	}

	// Handle as a regular query
	updater := NewSlackUpdater(mh.connection.client, ev.Channel, NewSlackFormatter())
	queryError := mh.queryHandler.HandleQueryWithUpdater(ctx, slackCtx, session, cleanMessage, updater)
	return errors.Join(err, queryError)
}

// LogSessionEvent logs a session event
func (mh *MessageHandler) LogSessionEvent(userID, channelID, event string) {
	mh.securityLogger.LogSessionEvent(userID, channelID, event)
}
