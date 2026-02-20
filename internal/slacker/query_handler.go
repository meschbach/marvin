package slacker

import (
	"context"
	"errors"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/ollama/ollama/api"
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
	streamer       *QueryStreamer[conversation.LLM]
	helpIntegrator *HelpIntegrator
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
	helpIntegrator *HelpIntegrator,
) (*QueryProcessor, error) {
	llm, err := query.NewLLM(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LLM: %w", err)
	}
	streamer := NewQueryStreamer(tenantToolSet, sessionManager, config, securityLogger, formatter, llm, helpIntegrator)
	return &QueryProcessor{
		tenantToolSet:  tenantToolSet,
		sessionManager: sessionManager,
		connection:     connection,
		config:         config,
		securityLogger: securityLogger,
		formatter:      formatter,
		streamer:       streamer,
		helpIntegrator: helpIntegrator,
	}, nil
}

// HandleQueryWithUpdater processes queries with a specific Slack updater
func (qp *QueryProcessor) HandleQueryWithUpdater(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, updater *SlackUpdater) error {
	// Add user message to session
	userMsg := api.Message{Role: "user", Content: message}
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

	userToolSet, deniedTools, err := qp.tenantToolSet.GetUserToolsWithDeniedInfo(ctx, userCtx)
	if err != nil {
		return fmt.Errorf("getting user tools: %w", err)
	}

	// Provide help if tools were denied access
	if len(deniedTools) > 0 && qp.helpIntegrator != nil {
		go qp.provideToolAccessHelp(ctx, slackCtx, deniedTools)
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

// provideToolAccessHelp provides intelligent help when tool access is denied
func (qp *QueryProcessor) provideToolAccessHelp(ctx context.Context, slackCtx *SlackContext, deniedTools []string) {
	if qp.helpIntegrator == nil {
		return
	}

	// Analyze the first denied tool (for simplicity - could enhance to handle multiple)
	if len(deniedTools) > 0 {
		toolName := deniedTools[0]
		reason := "Permission denied - tool not available for your user account"

		analysis, err := qp.helpIntegrator.HandleToolAccessDenied(ctx, slackCtx.UserID, slackCtx.ChannelID, toolName, reason)
		if err != nil {
			qp.securityLogger.LogError(slackCtx.UserID, "help_system",
				fmt.Sprintf("Failed to provide tool access help: %v", err))
			return
		}

		// Only show help if confidence is above threshold
		if !ShouldShowHelp(analysis) {
			return
		}

		// Create help response
		helpResponse := qp.helpIntegrator.CreateHelpResponse(analysis)

		// Log the help for now - in a full implementation we'd send it via the notification system
		qp.securityLogger.LogInfo(slackCtx.UserID, "help_system",
			fmt.Sprintf("Tool access help prepared (confidence: %.2f): %s", analysis.Confidence, helpResponse.QuickText))
	}
}

// sendErrorMessage sends an error message to the specified channel
func (qp *QueryProcessor) sendErrorMessage(ctx context.Context, channelID, message string) error {
	if qp.connection == nil || qp.connection.client == nil {
		return nil
	}
	_, _, err := qp.connection.client.PostMessageContext(
		ctx,
		channelID,
		slack.MsgOptionText(message, true),
	)
	return err
}
