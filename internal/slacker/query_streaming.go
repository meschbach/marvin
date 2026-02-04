package slacker

import (
	"context"
	"fmt"
	"strings"

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

// ProcessQueryWithProgressiveResponse handles AI processing with progressive Slack updates
func (qs *QueryStreamer) ProcessQueryWithProgressiveResponse(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, userToolSet *query.ToolSet) {
	qs.ProcessQueryWithUpdater(ctx, slackCtx, session, message, userToolSet, nil)
}

// ProcessQueryWithUpdater handles AI processing with a specific Slack updater
func (qs *QueryStreamer) ProcessQueryWithUpdater(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, userToolSet *query.ToolSet, updater *SlackUpdater) {
	fmt.Printf("Starting message\n")

	// Create Ollama client
	client, err := api.ClientFromEnvironment()
	if err != nil {
		qs.securityLogger.LogError(slackCtx.UserID, "QueryStreamer", fmt.Sprintf("Error creating Ollama client: %v", err))
		return
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

	// Process streaming response
	var assistantContent, thinkingBuffer strings.Builder
	var pendingCalls []api.ToolCall
	var thisLine strings.Builder
	var thisThinking strings.Builder

	err = client.Chat(ctx, req, func(resp api.ChatResponse) error {
		if resp.Done {
			// Response complete - would mark updater as complete
		}

		// Handle content
		if s := resp.Message.Content; s != "" {
			thisLine.WriteString(s)
			if strings.Contains(s, "\n") {
				assistantContent.WriteString(thisLine.String())
				thisLine.Reset()

				// Update Slack message if updater is available
				if updater != nil {
					updater.AddContent(thisLine.String())
				} else {
					qs.simulateSlackUpdate(slackCtx.UserID, thisLine.String())
				}
			}
		}

		// Handle thinking
		if len(resp.Message.Thinking) > 0 {
			thisThinking.WriteString(resp.Message.Thinking)
			thinkingBuffer.WriteString(resp.Message.Thinking)

			if strings.Contains(resp.Message.Thinking, "\n") {
				// Update Slack with thinking if updater is available
				if updater != nil {
					updater.AddThought(thisThinking.String())
				} else {
					qs.simulateSlackUpdate(slackCtx.UserID, thisThinking.String())
				}
				thisThinking.Reset()
			}
		}

		// Handle tool calls
		if len(resp.Message.ToolCalls) > 0 {
			pendingCalls = append(pendingCalls, resp.Message.ToolCalls...)
			// Log tool calls in Slack update
			for _, toolCall := range resp.Message.ToolCalls {
				if updater != nil {
					updater.AddToolCall(toolCall.Function.Name)
				} else {
					qs.simulateSlackUpdate(slackCtx.UserID, fmt.Sprintf("Tool called: %s", toolCall.Function.Name))
				}
			}
		}

		return nil
	})

	// Handle any remaining content
	if thisLine.Len() > 0 {
		assistantContent.WriteString(thisLine.String())
	}
	if thisThinking.Len() > 0 {
		thinkingBuffer.WriteString(thisThinking.String())
	}

	// Handle chat errors
	if err != nil {
		qs.securityLogger.LogError(slackCtx.UserID, "QueryStreamer", fmt.Sprintf("Error in Ollama chat: %v", err))
		return
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
	finalContent := assistantContent.String()
	if thinkingBuffer.Len() > 0 {
		finalContent += fmt.Sprintf("\n\n> Thought: %s", thinkingBuffer.String())
	}

	assistantMsg := api.Message{
		Role:      "assistant",
		Content:   finalContent,
		ToolCalls: pendingCalls,
		Thinking:  thinkingBuffer.String(),
	}
	if err := qs.sessionManager.AddMessage(slackCtx.UserID, slackCtx.ChannelID, assistantMsg); err != nil {
		qs.securityLogger.LogError(slackCtx.UserID, "SessionManager", err.Error())
	}
}

// simulateSlackUpdate simulates Slack message updates (placeholder implementation)
func (qs *QueryStreamer) simulateSlackUpdate(userID, content string) {
	// In a real implementation, this would update Slack message
	// For now, just log to console
	fmt.Printf("[Slack Update for %s]: %s\n", userID, strings.TrimSpace(content))
}
