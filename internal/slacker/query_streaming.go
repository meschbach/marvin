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
) *QueryStreamer {
	return &QueryStreamer{
		tenantToolSet:   tenantToolSet,
		sessionManager:  sessionManager,
		config:          config,
		securityLogger:  securityLogger,
		formatter:       formatter,
		languageService: languageService,
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

		// Fall back to default model if access is denied
		model = config.DefaultLanguageModel
		qs.securityLogger.LogInfo(slackCtx.UserID, "model_access",
			fmt.Sprintf("Model fallback: requested=%s, fallback=%s", qs.config.LanguageModel(), model))
	}

	return engine.RunConversation(ctx, model, updater)
}
