package slacker

import (
	"context"
	"errors"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/ollama/ollama/api"
)

type llm interface {
	Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error
}

// QueryStreamer handles LLM integration and streaming responses
type QueryStreamer[LLM llm] struct {
	tenantToolSet   *query.TenantToolSet
	sessionManager  *SessionManager
	config          *config.File
	securityLogger  *sec.SecurityLogger
	languageService LLM

	//Deprecated: not used
	formatter *SlackFormatter
}

// NewQueryStreamer creates a new query streamer
func NewQueryStreamer[LLM llm](
	tenantToolSet *query.TenantToolSet,
	sessionManager *SessionManager,
	config *config.File,
	securityLogger *sec.SecurityLogger,
	formatter *SlackFormatter,
	languageService LLM,
) *QueryStreamer[LLM] {
	return &QueryStreamer[LLM]{
		tenantToolSet:   tenantToolSet,
		sessionManager:  sessionManager,
		config:          config,
		securityLogger:  securityLogger,
		formatter:       formatter,
		languageService: languageService,
	}
}

// ProcessQueryWithUpdater handles AI processing with a specific Slack updater using the unified ConversationEngine
func (qs *QueryStreamer[LLM]) ProcessQueryWithUpdater(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, userToolSet *query.ToolSet, updater *SlackUpdater) error {
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
	llmAdapter := NewSlackLLMAdapter(qs.languageService)
	loggerAdapter := NewSlackLoggerAdapter(qs.securityLogger, slackCtx.UserID)

	// Create message callback for session management
	messageCallback := func(ctx context.Context, msg api.Message) error {
		return qs.sessionManager.AddMessage(slackCtx.UserID, slackCtx.ChannelID, msg)
	}

	// Create the conversation engine with callback
	engine := query.NewConversationEngineWithCallback(
		llmAdapter,
		qs.config,
		loggerAdapter,
		userToolSet,
		messages,
		messageCallback,
	)

	// Run the conversation using the unified engine
	model := qs.config.LanguageModel()
	return engine.RunConversation(ctx, model, updater)
}
