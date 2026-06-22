package slacker

import (
	"context"
	"errors"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/llm"
	llmfactory "github.com/meschbach/marvin/internal/llm/factory"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// QueryStreamer handles LLM integration and streaming responses
type QueryStreamer struct {
	tenantToolSet  *query.TenantToolSet
	sessionManager *SessionManager
	config         *config.File
	securityLogger *sec.SecurityLogger

	// testLLM overrides the factory-created chain for testing
	testLLM conversation.LLM

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
) *QueryStreamer {
	return &QueryStreamer{
		tenantToolSet:  tenantToolSet,
		sessionManager: sessionManager,
		config:         config,
		securityLogger: securityLogger,
		formatter:      formatter,
	}
}

// WithTestLLM sets a mock LLM for testing purposes.
func (qs *QueryStreamer) WithTestLLM(llm conversation.LLM) *QueryStreamer {
	qs.testLLM = llm
	return qs
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
	messages := []llm.Message{
		{Role: "system", Content: systemMessageContent},
	}
	messages = append(messages, session.Messages...)
	messages = append(messages, llm.Message{Role: "user", Content: message})

	// Create LLM adapter and logger adapter
	loggerAdapter := NewSlackLoggerAdapter(qs.securityLogger, slackCtx.UserID)

	// Create LLM chain with user-specific access control
	var llmClient conversation.LLM
	if qs.testLLM != nil {
		llmClient = qs.testLLM
	} else {
		accessCheck := func(model string) bool {
			allowed, _ := qs.config.ValidateModelAccess(model, slackCtx.UserID)
			return allowed
		}
		var err error
		llmClient, err = llmfactory.NewFromConfig(ctx, qs.config, accessCheck)
		if err != nil {
			return fmt.Errorf("failed to create LLM chain: %w", err)
		}
	}

	// Create message callback for session management
	messageCallback := func(ctx context.Context, msg llm.Message) error {
		return qs.sessionManager.AddMessage(slackCtx.UserID, slackCtx.ChannelID, msg)
	}

	// Create the conversation engine with callback
	engine := conversation.NewEngineWithCallback(
		llmClient,
		qs.config,
		loggerAdapter,
		userToolSet,
		messages,
		messageCallback,
	)

	// Get the requested model
	model := qs.config.LanguageModel()

	return engine.RunConversation(ctx, model, updater)
}
