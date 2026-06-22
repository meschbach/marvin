package slacker

import (
	"context"
	"errors"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/llm"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/slack-go/slack"
)

// QueryProcessor handles AI queries with streaming responses
type QueryProcessor struct {
	tenantToolSet  *query.TenantToolSet
	sessionManager *SessionManager
	connection     *SlackConnection
	config         *config.File
	securityLogger *sec.SecurityLogger
	formatter      *SlackFormatter
	streamer       *QueryStreamer
}

// QueryProcessorImpl maintains backward compatibility
type QueryProcessorImpl = QueryProcessor

// NewQueryProcessor creates a new query processor
func NewQueryProcessor(
	tenantToolSet *query.TenantToolSet,
	sessionManager *SessionManager,
	connection *SlackConnection,
	config *config.File,
	securityLogger *sec.SecurityLogger,
	formatter *SlackFormatter,
) (*QueryProcessor, error) {
	streamer := NewQueryStreamer(tenantToolSet, sessionManager, config, securityLogger, formatter)
	return &QueryProcessor{
		tenantToolSet:  tenantToolSet,
		sessionManager: sessionManager,
		connection:     connection,
		config:         config,
		securityLogger: securityLogger,
		formatter:      formatter,
		streamer:       streamer,
	}, nil
}

// HandleQueryWithUpdater processes queries with a specific Slack updater
func (qp *QueryProcessor) HandleQueryWithUpdater(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, updater *SlackUpdater) error {
	// Add user message to session
	userMsg := llm.Message{Role: "user", Content: message}
	if err := qp.sessionManager.AddMessage(slackCtx.UserID, slackCtx.ChannelID, userMsg); err != nil {
		qp.securityLogger.LogError(slackCtx.UserID, "SessionManager", err.Error())
		return err
	}

	// Get user's tools
	userCtx := &query.UserContext{
		UserID:      slackCtx.UserID,
		SlackTeamID: slackCtx.TeamID,
		IsAdmin:     qp.tenantToolSet.IsAdmin(slackCtx.UserID),
	}

	userToolSet, _, err := qp.tenantToolSet.GetUserToolsWithDeniedInfo(ctx, userCtx)
	if err != nil {
		return fmt.Errorf("getting user tools: %w", err)
	}

	// Start progressive response with specific updater
	go func(ctx context.Context) {
		ctx, done := context.WithCancel(ctx)
		defer done()
		defer func() {
			err := updater.ForceUpdate(ctx)
			if err != nil {
				qp.securityLogger.LogError(slackCtx.UserID, "Updater ForceUpdate", err.Error())
			}
			fmt.Printf("(%s) query complete\n", slackCtx.UserID)
		}()

		err := qp.streamer.ProcessQueryWithUpdater(ctx, slackCtx, session, message, userToolSet, updater)
		if err != nil {
			qp.securityLogger.LogError(slackCtx.UserID, "QueryProcessing", err.Error())

			errorMsg := "⚠️ I encountered an error processing your request. Please try again."
			var llmErr *conversation.LLMConnectionError
			if errors.As(err, &llmErr) {
				errorMsg = "⚠️ I'm having trouble connecting to the AI service. Please try again in a moment."
			} else if errors.Is(err, context.DeadlineExceeded) {
				errorMsg = "⏱️ Your request took too long. Please try a simpler query."
			}

			if sendErr := qp.sendErrorMessage(ctx, slackCtx.ChannelID, errorMsg); sendErr != nil {
				qp.securityLogger.LogError(slackCtx.UserID, "ErrorNotification", sendErr.Error())
			}
		}
	}(ctx)

	return nil
}

// sendErrorMessage sends an error message to the specified channel
func (qp *QueryProcessor) sendErrorMessage(ctx context.Context, channelID, message string) error {
	if qp.connection == nil || qp.connection.GetClient() == nil {
		return nil
	}
	_, _, err := qp.connection.GetClient().PostMessageContext(
		ctx,
		channelID,
		slack.MsgOptionText(message, true),
	)
	return err
}
