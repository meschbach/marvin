package conversation

import (
	"context"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/junk"
	"github.com/ollama/ollama/api"
)

// Engine handles the core AI conversation loop with Tool-call handling.
// It extracts the conversation logic from ollamaConversation while providing
// pluggable interfaces for different streaming backends and optional logging.
type Engine struct {
	// client is the LLM provider for making API calls
	client LLM

	// config contains model options and system prompts
	config *config.File

	// logger provides optional debugging and security logging
	logger Logger

	// tools represent the available tools for the conversation
	tools *ToolSet

	// messages maintains the conversation history
	messages []api.Message

	// messageCallback enables integration with external systems
	messageCallback MessageCallback
}

// Config provides configuration options for the conversation engine
type Config struct {
	Logger Logger // Defaults to NullLogger if not provided
	UserID string // For security logging
}

// NewEngine creates a new conversation engine with the provided components
func NewEngine(
	client LLM,
	configuration *config.File,
	logger Logger,
	tools *ToolSet,
	messages []api.Message,
) *Engine {
	if logger == nil {
		logger = &NullLogger{}
	}

	return &Engine{
		client:   client,
		config:   configuration,
		logger:   logger,
		tools:    tools,
		messages: messages,
	}
}

// NewEngineWithCallback creates a conversation engine with a message callback.
func NewEngineWithCallback(
	client LLM,
	configuration *config.File,
	logger Logger,
	tools *ToolSet,
	messages []api.Message,
	messageCallback MessageCallback,
) *Engine {
	if logger == nil {
		logger = &NullLogger{}
	}

	return &Engine{
		client:          client,
		config:          configuration,
		logger:          logger,
		tools:           tools,
		messages:        messages,
		messageCallback: messageCallback,
	}
}

// RunConversation executes the AI chat loop with Tool-call handling until
// the assistant produces a final answer (no further Tool calls) or an error occurs.
func (e *Engine) RunConversation(
	ctx context.Context,
	model string,
	updater StreamingUpdater,
) error {
	ctx, span := tracer.Start(ctx, "Engine.RunConversation")
	defer span.End()

	updater = WrapWithOptionalStats(updater)

	for {
		span.AddEvent("turn.execute")
		turnResult, err := e.executeTurn(ctx, model, updater)
		if err != nil {
			return junk.RecordSpanError(span, err)
		}

		span.AddEvent("turn.complete")
		if len(turnResult.PendingCalls) == 0 {
			return junk.MaybeRecordSpanError(span, updater.Flush(ctx))
		}

		_, err = e.executeToolCalls(ctx, turnResult.PendingCalls, updater)
		if err != nil {
			return junk.RecordSpanError(span, err)
		}
	}
}
