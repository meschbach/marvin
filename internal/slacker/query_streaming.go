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

// QueryStreamer handles LLM integration and streaming responses
type QueryStreamer struct {
	tenantToolSet  *query.TenantToolSet
	sessionManager *SessionManager
	config         *config.File
	securityLogger *sec.SecurityLogger
	formatter      *SlackFormatter
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

// ProcessQueryWithUpdater handles AI processing with a specific Slack updater
func (qs *QueryStreamer) ProcessQueryWithUpdater(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, userToolSet *query.ToolSet, updater *SlackUpdater) error {
	fmt.Printf("Starting message\n")
	if updater == nil { //catch here to avoid costly
		return errors.New("updater is required")
	}

	// Create Ollama client
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return err
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
	}

	// Process streaming response with extracted handler
	handler := newStreamingResponseHandler(updater)

	err = client.Chat(ctx, req, handler.handleResponse)

	// Extract final state from handler
	assistantContent, thinkingBuffer, pendingCalls := handler.finished()

	// Handle chat errors
	if err != nil {
		return err
	}

	// Execute tool calls if any
	if len(pendingCalls) > 0 {
		for _, call := range pendingCalls {
			reply, herr := userToolSet.HandleCall(ctx, call)
			if herr != nil {
				qs.securityLogger.LogError(slackCtx.UserID, "QueryStreamer", fmt.Sprintf("Error invoking tool %s: %v", call.Function.Name, herr))
				continue
			}
			messages = append(messages, reply...)
		}
	}

	// Add assistant message to session
	finalContent := assistantContent
	if thinkingBuffer != "" {
		finalContent += fmt.Sprintf("\n\n> Thought: %s", thinkingBuffer)
	}

	assistantMsg := api.Message{
		Role:      "assistant",
		Content:   finalContent,
		ToolCalls: pendingCalls,
		Thinking:  thinkingBuffer,
	}
	if err := qs.sessionManager.AddMessage(slackCtx.UserID, slackCtx.ChannelID, assistantMsg); err != nil {
		return err
	}
	return nil
}
