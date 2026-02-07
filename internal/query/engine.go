package query

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
)

// Role constants used throughout the conversation engine and related components.
// These provide centralized, well-documented role definitions.

const (
	// RoleSystem represents system messages that set context and behavior
	RoleSystem = "system"

	// RoleUser represents user input messages
	RoleUser = "user"

	// RoleAssistant represents AI-generated responses
	RoleAssistant = "assistant"

	// RoleToolResult represents tool execution results
	RoleToolResult = "tool_result"
)

// MessageCallback is called when a new message should be added to the conversation.
// This enables integration with external systems like Slack's session management.
type MessageCallback func(ctx context.Context, msg api.Message) error

// ConversationEngine handles the core AI conversation loop with tool-call handling.
// It extracts the conversation logic from ollamaConversation while providing
// pluggable interfaces for different streaming backends and optional logging.
type ConversationEngine struct {
	// client is the LLM provider for making API calls
	client LLM

	// config contains model options and system prompts
	config *config.File

	// logger provides optional debugging and security logging
	logger Logger

	// tools represents the available tools for the conversation
	tools *ToolSet

	// messages maintains the conversation history
	messages []api.Message

	// messageCallback enables integration with external systems
	messageCallback MessageCallback
}

// EngineConfig provides configuration options for the conversation engine
type EngineConfig struct {
	Logger Logger // Defaults to NullLogger if not provided
	UserID string // For security logging
}

// NewConversationEngine creates a new conversation engine with the provided components
func NewConversationEngine(
	client LLM,
	config *config.File,
	logger Logger,
	tools *ToolSet,
	messages []api.Message,
) *ConversationEngine {
	if logger == nil {
		logger = &NullLogger{}
	}

	return &ConversationEngine{
		client:   client,
		config:   config,
		logger:   logger,
		tools:    tools,
		messages: messages,
	}
}

// NewConversationEngineWithCallback creates a conversation engine with a message callback
func NewConversationEngineWithCallback(
	client LLM,
	config *config.File,
	logger Logger,
	tools *ToolSet,
	messages []api.Message,
	messageCallback MessageCallback,
) *ConversationEngine {
	if logger == nil {
		logger = &NullLogger{}
	}

	return &ConversationEngine{
		client:          client,
		config:          config,
		logger:          logger,
		tools:           tools,
		messages:        messages,
		messageCallback: messageCallback,
	}
}

// RunConversation executes the AI chat loop with tool-call handling until
// the assistant produces a final answer (no further tool calls) or an error occurs.
func (e *ConversationEngine) RunConversation(
	ctx context.Context,
	model string,
	updater StreamingUpdater,
) error {
	// Wrap updater with optional statistics support if needed
	updater = WrapWithOptionalStats(updater)

	for {
		stream := true
		req := &api.ChatRequest{
			Model:    model,
			Messages: e.messages,
			Tools:    e.tools.APITools(),
			Stream:   &stream,
			Options:  e.config.BuildAPIOptions(),
		}

		// Track statistics and response for this turn
		var (
			assistantOut, thinkingBuffer strings.Builder
			thisLine, thisThinking       strings.Builder
			pendingCalls                 []api.ToolCall
			cumulativeStats              ConversationStats
		)

		err := e.client.Chat(ctx, req, func(resp api.ChatResponse) error {
			// Update statistics in real-time
			if resp.Done {
				cumulativeStats.IsDone = true
				cumulativeStats.EvalCount = resp.EvalCount
				cumulativeStats.DoneReason = resp.DoneReason
				cumulativeStats.ResponseTokens += resp.EvalCount
				cumulativeStats.PromptTokens += resp.PromptEvalCount
				cumulativeStats.TotalTokens = cumulativeStats.PromptTokens + cumulativeStats.ResponseTokens

				// Send real-time stats to updater
				if statsErr := updater.UpdateStats(ctx, cumulativeStats); statsErr != nil {
					return &StreamingUpdateError{
						Component: "ConversationEngine.UpdateStats",
						Message:   "failed to update statistics",
						Cause:     statsErr,
					}
				}
			}

			// Handle content streaming
			if s := resp.Message.Content; s != "" {
				thisLine.WriteString(s)
				if strings.Contains(s, "\n") {
					if err := updater.AddContent(ctx, thisLine.String()); err != nil {
						return &StreamingUpdateError{
							Component: "ConversationEngine.AddContent",
							Message:   "failed to stream content",
							Cause:     err,
						}
					}
					thisLine.Reset()
				}
				assistantOut.WriteString(s)
			}

			// Handle thinking content
			if len(resp.Message.Thinking) > 0 {
				thisThinking.WriteString(resp.Message.Thinking)
				// Note: We'll flush thinking on state transitions like the Slack implementation
			}

			// Handle tool calls
			if len(resp.Message.ToolCalls) > 0 {
				pendingCalls = append(pendingCalls, resp.Message.ToolCalls...)
				// Log tool calls for debugging
				e.logger.Debug("", "ConversationEngine", fmt.Sprintf("Tool call detected: %s", resp.Message.ToolCalls[0].Function.Name))
				for _, toolCall := range resp.Message.ToolCalls {
					if err := updater.AddToolCall(ctx, toolCall.Function.Name); err != nil {
						return &StreamingUpdateError{
							Component: "ConversationEngine.AddToolCall",
							Message:   fmt.Sprintf("failed to stream tool call for %s", toolCall.Function.Name),
							Cause:     err,
						}
					}
				}
			}

			return nil
		})

		if err != nil {
			e.logger.Error("", "ConversationEngine", fmt.Sprintf("Error querying LLM: %v", err))
			return &LLMConnectionError{
				Message: "LLM chat request failed",
				Cause:   err,
			}
		}

		// Flush any remaining content and thinking
		if thisLine.Len() > 0 {
			if err := updater.AddContent(ctx, thisLine.String()); err != nil {
				return &StreamingUpdateError{
					Component: "ConversationEngine.FlushContent",
					Message:   "failed to flush remaining content",
					Cause:     err,
				}
			}
		}
		if thisThinking.Len() > 0 {
			if err := updater.AddThought(ctx, thisThinking.String()); err != nil {
				return &StreamingUpdateError{
					Component: "ConversationEngine.FlushThinking",
					Message:   "failed to flush thinking content",
					Cause:     err,
				}
			}
			thinkingBuffer.WriteString(thisThinking.String())
		}

		// Record the assistant turn (including tool calls, if any)
		assistantMsg := api.Message{
			Role:      RoleAssistant,
			Content:   assistantOut.String(),
			ToolCalls: pendingCalls,
			Thinking:  thinkingBuffer.String(),
		}
		e.messages = append(e.messages, assistantMsg)

		// Call message callback if provided
		if e.messageCallback != nil {
			if err := e.messageCallback(ctx, assistantMsg); err != nil {
				e.logger.Error("", "ConversationEngine", fmt.Sprintf("Error in message callback: %v", err))
				return &MessageCallbackError{
					Operation: "messageCallback",
					Message:   "message callback failed",
					Cause:     err,
				}
			}
		}

		// If there are no tool calls, we are done for this turn
		if len(pendingCalls) == 0 {
			return updater.Flush(ctx)
		}

		// Execute all pending tool calls and collect responses
		var pendingCallsErrors error
		for _, call := range pendingCalls {
			e.logger.Debug("", "ConversationEngine", fmt.Sprintf("Invoking tool %s", call.Function.Name))
			reply, herr := e.tools.HandleCall(ctx, call)
			if herr != nil {
				e.logger.Error("", "ConversationEngine", fmt.Sprintf("Error invoking tool %s: %v", call.Function.Name, herr))
				wrappedErr := &ToolInvocationError{
					ToolName: call.Function.Name,
					Message:  "tool execution failed",
					Cause:    herr,
				}
				pendingCallsErrors = errors.Join(wrappedErr, pendingCallsErrors)
			}
			e.messages = append(e.messages, reply...)
		}
		if pendingCallsErrors != nil {
			return pendingCallsErrors
		}

		// Loop continues: the next iteration sends messages including tool outputs
	}
}
