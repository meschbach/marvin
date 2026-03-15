package slacker

import (
	"context"
	"fmt"
	"strings"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"go.opentelemetry.io/otel/attribute"
)

// SlackBot represents the main Slack bot orchestrator
type SlackBot struct {
	connection         *SlackConnection
	eventRouter        *EventRouter
	messageHandler     *MessageHandler
	queryProcessor     *QueryProcessorImpl
	toolManager        *ToolManagerImpl
	notificationSender *NotificationSender
	formatter          *SlackFormatter
	sessionManager     *SessionManager
	securityLogger     *sec.SecurityLogger
}

// NewSlackBot creates a new Slack bot instance with composed components
func NewSlackBot(
	slackToken string,
	appToken string,
	config *config.File,
	sessionManager *SessionManager,
	tenantToolSet *query.TenantToolSet,
	approvalWorkflow *ApprovalWorkflow,
	securityLogger *sec.SecurityLogger,
) (*SlackBot, error) {
	// Create Slack connection
	connection, err := NewSlackConnection(slackToken, appToken, config, securityLogger)
	if err != nil {
		return nil, fmt.Errorf("creating Slack connection: %w", err)
	}

	// Create supporting components
	formatter := NewSlackFormatter()
	notificationSender := NewNotificationSender(connection.GetClient(), connection.GetAdminUsers())

	// Inject dependencies into approval workflow
	approvalWorkflow.SetNotificationSender(notificationSender)
	approvalWorkflow.SetSessionManager(sessionManager)

	// Create message handler dependencies
	intentProcessor := NewIntentProcessor()
	queryHandler, err := NewQueryProcessor(tenantToolSet, sessionManager, connection, config, securityLogger, formatter, nil)
	if err != nil {
		return nil, fmt.Errorf("creating query processor: %w", err)
	}
	toolManager := NewToolManager(approvalWorkflow, tenantToolSet, securityLogger, notificationSender, sessionManager, nil)

	messageHandler := NewMessageHandler(
		intentProcessor,
		connection,
		queryHandler,
		toolManager,
		sessionManager,
		securityLogger,
		config,
		tenantToolSet,
	)

	// Create event router
	eventRouter := NewEventRouter(connection, messageHandler, securityLogger)

	// Create main orchestrator
	bot := &SlackBot{
		connection:         connection,
		eventRouter:        eventRouter,
		messageHandler:     messageHandler,
		queryProcessor:     queryHandler,
		toolManager:        toolManager,
		notificationSender: notificationSender,
		formatter:          formatter,
		sessionManager:     sessionManager,
		securityLogger:     securityLogger,
	}

	// Set approval workflow notification function
	approvalWorkflow.SetNotifyFunction(func(ctx context.Context, request *ToolApprovalRequest) error {
		return bot.notifyAdmins(ctx, request)
	})

	return bot, nil
}

// StartSocketMode starts the Socket Mode API
func (sb *SlackBot) StartSocketMode(ctx context.Context) error {
	go func() {
		if err := sb.connection.socketClient.RunContext(ctx); err != nil {
			sb.handleConnectionError(err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-sb.connection.socketClient.Events:
			spanName := fmt.Sprintf("slacker.event.%s", event.Type)
			ctx, span := tracer.Start(ctx, spanName)

			// Extract common attributes from event
			var attrs []attribute.KeyValue
			switch event.Type {
			case "events_api":
				if payload, ok := event.Data.(map[string]interface{}); ok {
					if channel, ok := payload["channel"].(string); ok {
						attrs = append(attrs, attribute.String("channel", channel))
					}
					if user, ok := payload["user"].(string); ok {
						attrs = append(attrs, attribute.String("user", user))
					}
					if eventType, ok := payload["type"].(string); ok {
						attrs = append(attrs, attribute.String("event_type", eventType))
					}
				}
			case "interactive":
				if payload, ok := event.Data.(map[string]interface{}); ok {
					if callbackID, ok := payload["callback_id"].(string); ok {
						attrs = append(attrs, attribute.String("callback_id", callbackID))
					}
					if user, ok := payload["user"].(map[string]interface{}); ok {
						if userID, ok := user["id"].(string); ok {
							attrs = append(attrs, attribute.String("user", userID))
						}
					}
				}
			case "slash_commands":
				if payload, ok := event.Data.(map[string]interface{}); ok {
					if command, ok := payload["command"].(string); ok {
						attrs = append(attrs, attribute.String("command", command))
					}
					if userID, ok := payload["user_id"].(string); ok {
						attrs = append(attrs, attribute.String("user", userID))
					}
				}
			}
			span.SetAttributes(attrs...)

			if err := sb.eventRouter.RouteEvent(ctx, event); err != nil {
				span.RecordError(err)
				botUserID := sb.connection.GetBotUserID()
				// Determine user ID for error logging - this is simplified
				if event.Type == "events_api" {
					// Would extract user from event in real implementation
					botUserID = "unknown"
				}
				sb.securityLogger.LogError(botUserID, "SlackBot", err.Error())
			}
			span.End()
		}
	}
}

// ValidateSlackSetup validates the Slack bot configuration and connections
//
//nolint:gocyclo
func (sb *SlackBot) ValidateSlackSetup() error {
	// Validate that all required components are initialized
	if sb.connection == nil {
		return fmt.Errorf("slack connection not initialized")
	}
	if sb.eventRouter == nil {
		return fmt.Errorf("event router not initialized")
	}
	if sb.messageHandler == nil {
		return fmt.Errorf("message handler not initialized")
	}
	if sb.toolManager == nil {
		return fmt.Errorf("tool manager not initialized")
	}
	if sb.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	if sb.securityLogger == nil {
		return fmt.Errorf("security logger not initialized")
	}

	// Validate connection details
	client := sb.connection.GetClient()
	if client == nil {
		return fmt.Errorf("slack client not available")
	}

	// Test connection by attempting to get bot info
	botUserID := sb.connection.GetBotUserID()
	if botUserID == "" {
		return fmt.Errorf("bot user ID not available")
	}

	// Validate admin users configuration
	adminUsers := sb.connection.GetAdminUsers()
	if len(adminUsers) == 0 {
		return fmt.Errorf("no admin users configured")
	}

	return nil
}

// GetConnection returns the Slack connection
func (sb *SlackBot) GetConnection() *SlackConnection {
	return sb.connection
}

// GetQueryProcessor returns the query processor
func (sb *SlackBot) GetQueryProcessor() *QueryProcessor {
	return sb.queryProcessor
}

// GetSessionManager returns the session manager
func (sb *SlackBot) GetSessionManager() *SessionManager {
	return sb.sessionManager
}

// notifyAdmins sends approval notifications to admin users
func (sb *SlackBot) notifyAdmins(ctx context.Context, request *ToolApprovalRequest) error {
	return sb.notificationSender.NotifyAdmins(ctx, request)
}

// handleConnectionError handles Socket Mode connection errors with enhanced diagnostics
func (sb *SlackBot) handleConnectionError(err error) {
	// Provide enhanced error diagnostics for Socket Mode failures
	errMsg := err.Error()

	// Check for specific authentication errors
	if strings.Contains(errMsg, "not_authed") {
		sb.securityLogger.LogError("system", "SlackSocketMode", "Authentication failed - check SLACK_APP_TOKEN and Socket Mode app configuration")
		fmt.Printf("❌ Socket Mode Authentication Error: 'not_authed'\n")
		fmt.Printf("   This typically means:\n")
		fmt.Printf("   1. SLACK_APP_TOKEN is missing or invalid\n")
		fmt.Printf("   2. App token doesn't have 'socket:write' scope\n")
		fmt.Printf("   3. Socket Mode is not enabled in the Slack app\n")
		fmt.Printf("   4. App is not properly installed in the workspace\n")
	} else if strings.Contains(errMsg, "invalid_auth") {
		sb.securityLogger.LogError("system", "SlackSocketMode", "Invalid authentication - check SLACK_BOT_TOKEN permissions")
		fmt.Printf("❌ Bot Token Authentication Error: 'invalid_auth'\n")
	} else {
		sb.securityLogger.LogError("system", "SlackSocketMode", fmt.Sprintf("Connection error: %v", err))
		fmt.Printf("❌ Socket Mode Connection Error: %v\n", err)
	}
}
