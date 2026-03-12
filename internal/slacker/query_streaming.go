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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type llm interface {
	Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error
}

// QueryStreamer handles LLM integration and streaming responses
type QueryStreamer struct {
	tenantToolSet   *query.TenantToolSet
	sessionManager  *SessionManager
	config          *config.File
	securityLogger  *sec.SecurityLogger
	languageService conversation.LLM
	helpIntegrator  *HelpIntegrator

	//Deprecated: not used
	formatter *SlackFormatter
}

// NewQueryStreamer creates a new query streamer
func NewQueryStreamer(
	tenantToolSet *query.TenantToolSet,
	sessionManager *SessionManager,
	config *config.File,
	securityLogger *sec.SecurityLogger,
	formatter *SlackFormatter,
	languageService conversation.LLM,
	helpIntegrator *HelpIntegrator,
) *QueryStreamer {
	return &QueryStreamer{
		tenantToolSet:   tenantToolSet,
		sessionManager:  sessionManager,
		config:          config,
		securityLogger:  securityLogger,
		formatter:       formatter,
		languageService: languageService,
		helpIntegrator:  helpIntegrator,
	}
}

// ProcessQueryWithUpdater handles AI processing with a specific Slack updater using the unified Engine
func (qs *QueryStreamer) ProcessQueryWithUpdater(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, userToolSet *conversation.ToolSet, updater *SlackUpdater) error {
	ctx, span := tracer.Start(ctx, "QueryStreamer.ProcessQueryWithUpdater",
		trace.WithAttributes(
			attribute.String("user", slackCtx.UserID),
			attribute.String("channel", slackCtx.ChannelID),
			attribute.String("model", qs.config.LanguageModel()),
		))
	defer span.End()

	qs.securityLogger.LogDebug(slackCtx.UserID, "query_streaming", "Starting message streaming")
	if updater == nil { //catch here to avoid costly
		return errors.New("updater is required")
	}

	// Prepare conversation messages
	systemMessageContent := "You are a helpful assistant integrated with Slack."
	if qs.config.SystemPrompt != nil && len(qs.config.SystemPrompt.FromString) > 0 {
		systemMessageContent = qs.config.SystemPrompt.FromString
	}

	// Build conversation history from session
	messages := []api.Message{
		{Role: "system", Content: systemMessageContent},
	}
	messages = append(messages, session.Messages...)
	messages = append(messages, api.Message{Role: "user", Content: message})

	// Create LLM adapter and logger adapter
	loggerAdapter := NewSlackLoggerAdapter(qs.securityLogger, slackCtx.UserID)

	// Create message callback for session management
	messageCallback := func(ctx context.Context, msg api.Message) error {
		return qs.sessionManager.AddMessage(slackCtx.UserID, slackCtx.ChannelID, msg)
	}

	// Create the conversation engine with callback
	engine := conversation.NewEngineWithCallback(
		qs.languageService,
		qs.config,
		loggerAdapter,
		userToolSet,
		messages,
		messageCallback,
	)

	// Get the requested model and validate access for Slacker operations
	model := qs.config.LanguageModel()
	allowed, reason := qs.config.ValidateModelAccess(model, slackCtx.UserID)
	if !allowed {
		qs.securityLogger.LogError(slackCtx.UserID, "model_access",
			fmt.Sprintf("Model access denied: model=%s, reason=%s", model, reason))

		// Provide intelligent help for model access denial
		if qs.helpIntegrator != nil {
			go qs.provideModelAccessHelp(ctx, slackCtx, model, reason)
		}

		// Fall back to default model if access is denied
		model = config.DefaultLanguageModel
		qs.securityLogger.LogInfo(slackCtx.UserID, "model_access",
			fmt.Sprintf("Model fallback: requested=%s, fallback=%s", qs.config.LanguageModel(), model))
	}

	return engine.RunConversation(ctx, model, updater)
}

// provideModelAccessHelp provides intelligent help when model access is denied
func (qs *QueryStreamer) provideModelAccessHelp(ctx context.Context, slackCtx *SlackContext, model, reason string) {
	if qs.helpIntegrator == nil {
		return
	}

	// Analyze the model access failure
	analysis, err := qs.helpIntegrator.HandleModelAccessFailure(ctx, slackCtx.UserID, slackCtx.ChannelID, model, reason)
	if err != nil {
		qs.securityLogger.LogError(slackCtx.UserID, "help_system",
			fmt.Sprintf("Failed to provide model access help: %v", err))
		return
	}

	// Only show help if confidence is above threshold
	if !ShouldShowHelp(analysis) {
		return
	}

	// Create help response
	helpResponse := qs.helpIntegrator.CreateHelpResponse(analysis)

	// Send help message using PostMessageContext (similar to message_handler.go)
	// This is a simplified approach - in a full implementation we'd need access to the Slack client
	qs.securityLogger.LogInfo(slackCtx.UserID, "help_system",
		fmt.Sprintf("Model access help prepared (confidence: %.2f): %s", analysis.Confidence, helpResponse.QuickText))
}
