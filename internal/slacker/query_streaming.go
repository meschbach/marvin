package slacker

import (
	"context"
	"errors"
	"fmt"

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

// ProcessQueryWithUpdater handles AI processing with a specific Slack updater
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

	// Get available tools from user toolset
	availableTools := userToolSet.APITools()

	// Create streaming chat request
	stream := true
	req := &api.ChatRequest{
		Model:    qs.config.LanguageModel(),
		Messages: messages,
		Tools:    availableTools,
		Stream:   &stream,
		Options:  qs.config.BuildAPIOptions(),
	}

	// Process streaming response with extracted handler
	handler := newStreamingResponseHandler(updater)

	// Wrap handleResponse to provide context
	err := qs.languageService.Chat(ctx, req, func(resp api.ChatResponse) error {
		return handler.handleResponse(ctx, resp)
	})

	// Extract final state from handler
	assistantContent, thinkingBuffer, pendingCalls, err := handler.finished(ctx)

	// Handle chat errors
	if err != nil {
		return err
	}

	assistantMsg := api.Message{
		Role:      "assistant",
		Content:   assistantContent,
		ToolCalls: pendingCalls,
		Thinking:  thinkingBuffer,
	}
	if err := qs.sessionManager.AddMessage(slackCtx.UserID, slackCtx.ChannelID, assistantMsg); err != nil {
		return err
	}

	// Continue conversation loop while there are tool calls to process
	for len(pendingCalls) > 0 {
		if err := updater.ForceUpdate(ctx); err != nil {
			return err
		}
		// Execute all pending tool calls and collect responses
		for _, call := range pendingCalls {
			reply, herr := userToolSet.HandleCall(ctx, call)
			if herr != nil {
				qs.securityLogger.LogError(slackCtx.UserID, "QueryStreamer", fmt.Sprintf("Error invoking tool %s: %v", call.Function.Name, herr))
				// Add error message to conversation so LLM can respond appropriately
				errorMsg := api.Message{
					Role:       "tool",
					ToolName:   call.Function.Name,
					ToolCallID: call.ID,
					Content:    fmt.Sprintf("Error: %v", herr),
				}
				messages = append(messages, errorMsg)
				continue
			}
			messages = append(messages, reply...)
		}

		// Create a streaming request to let the LLM consume tool results
		nextHandler := newStreamingResponseHandler(updater)

		// Build the follow-up request with tool results included
		followUpReq := &api.ChatRequest{
			Model:    qs.config.LanguageModel(),
			Messages: messages,
			Stream:   &stream,
			Options:  qs.config.BuildAPIOptions(),
		}

		// Make LLM call to consume tool results and continue conversation
		err = qs.languageService.Chat(ctx, followUpReq, func(resp api.ChatResponse) error {
			return nextHandler.handleResponse(ctx, resp)
		})
		if err != nil {
			return err
		}

		// Extract response state
		nextAssistantContent, nextThinkingBuffer, nextPendingCalls, err := nextHandler.finished(ctx)
		if err != nil {
			return err
		}

		// Add this assistant message to session
		nextAssistantMsg := api.Message{
			Role:      "assistant",
			Content:   nextAssistantContent,
			ToolCalls: nextPendingCalls,
			Thinking:  nextThinkingBuffer,
		}
		if err := qs.sessionManager.AddMessage(slackCtx.UserID, slackCtx.ChannelID, nextAssistantMsg); err != nil {
			return err
		}

		// Update loop state for next iteration
		pendingCalls = nextPendingCalls
	}

	return nil
}
