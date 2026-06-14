package conversation

import (
	"context"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/llm"
)

// Runner executes conversations with injected dependencies.
type Runner struct {
	config       *config.File
	llmCreator   LLMCreator
	toolProvider ToolProvider
	logger       Logger
}

// LLMCreator creates an LLM for a given model.
type LLMCreator func(ctx context.Context, cfg *config.File, model string) (LLM, error)

// ToolProvider creates tool sets for a user.
type ToolProvider func(ctx context.Context, cfg *config.File, userID string, toolNames []string) (*ToolSet, error)

// NewRunner creates a new conversation runner.
func NewRunner(
	cfg *config.File,
	llmCreator LLMCreator,
	toolProvider ToolProvider,
) *Runner {
	return &Runner{
		config:       cfg,
		llmCreator:   llmCreator,
		toolProvider: toolProvider,
		logger:       &NullLogger{},
	}
}

// RunRequest contains all inputs for a conversation.
type RunRequest struct {
	UserID       string
	SystemPrompt string
	// Messages are pre-existing messages (history/session) excluding the final user message
	Messages []llm.Message
	// UserMessage is the current user message to process
	UserMessage string
	// ToolNames specifies which tools the agent should have access to
	ToolNames []string
	// Updater streams the conversation response
	Updater StreamingUpdater
	// Callback is invoked for each message during the conversation (optional)
	Callback MessageCallback
}

// RunResult contains the output of a conversation.
type RunResult struct {
	Content string
}

// Run executes the conversation and returns the final result.
func (r *Runner) Run(ctx context.Context, model string, req *RunRequest) (*RunResult, error) {
	// Create LLM
	client, err := r.llmCreator(ctx, r.config, model)
	if err != nil {
		return nil, fmt.Errorf("creating LLM: %w", err)
	}

	// Create tool set
	toolSet, err := r.toolProvider(ctx, r.config, req.UserID, req.ToolNames)
	if err != nil {
		return nil, fmt.Errorf("creating tools: %w", err)
	}

	// Build message list
	capacity := 1 + len(req.Messages) + 1 // system + existing + user
	messages := make([]llm.Message, 0, capacity)
	messages = append(messages, llm.Message{Role: "system", Content: req.SystemPrompt})
	messages = append(messages, req.Messages...)
	messages = append(messages, llm.Message{Role: "user", Content: req.UserMessage})

	// Create engine with callback if provided
	var engine *Engine
	if req.Callback != nil {
		engine = NewEngineWithCallback(client, r.config, r.logger, toolSet, messages, req.Callback)
	} else {
		engine = NewEngine(client, r.config, r.logger, toolSet, messages)
	}

	// Execute conversation
	err = engine.RunConversation(ctx, model, req.Updater)
	if err != nil {
		return nil, fmt.Errorf("running conversation: %w", err)
	}

	// Extract content from updater if it's a RecordingUpdater
	if recorder, ok := req.Updater.(*RecordingUpdater); ok {
		return &RunResult{Content: recorder.Content()}, nil
	}

	// For non-recording updaters, return empty content
	return &RunResult{Content: ""}, nil
}
