package slacker

import (
	"context"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/ollama/ollama/api"
)

// QueryProcessor handles AI queries with streaming responses
type QueryProcessor struct {
	tenantToolSet  *query.TenantToolSet
	sessionManager *SessionManager
	config         *config.File
	securityLogger *sec.SecurityLogger
	formatter      *SlackFormatter
	streamer       *QueryStreamer[*api.Client]
}

// QueryProcessorImpl maintains backward compatibility
type QueryProcessorImpl = QueryProcessor

// NewQueryProcessor creates a new query processor
func NewQueryProcessor(
	tenantToolSet *query.TenantToolSet,
	sessionManager *SessionManager,
	config *config.File,
	securityLogger *sec.SecurityLogger,
	formatter *SlackFormatter,
) *QueryProcessor {
	ollama, err := api.ClientFromEnvironment()
	if err != nil {
		//todo: handle more gracefully
		panic(err)
	}
	streamer := NewQueryStreamer(tenantToolSet, sessionManager, config, securityLogger, formatter, ollama)
	return &QueryProcessor{
		tenantToolSet:  tenantToolSet,
		sessionManager: sessionManager,
		config:         config,
		securityLogger: securityLogger,
		formatter:      formatter,
		streamer:       streamer,
	}
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

	userToolSet, err := qp.tenantToolSet.GetUserTools(ctx, userCtx)
	if err != nil {
		return fmt.Errorf("getting user tools: %w", err)
	}

	// Start progressive response with specific updater
	go func() {
		ctx, done := context.WithCancel(context.Background())
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
		}
	}()

	return nil
}
