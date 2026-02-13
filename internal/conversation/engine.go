package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/meschbach/marvin/internal/config"
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
	config *config.File,
	logger Logger,
	tools *ToolSet,
	messages []api.Message,
) *Engine {
	if logger == nil {
		logger = &NullLogger{}
	}

	return &Engine{
		client:   client,
		config:   config,
		logger:   logger,
		tools:    tools,
		messages: messages,
	}
}

// NewEngineWithCallback creates a conversation engine with a message callback
func NewEngineWithCallback(
	client LLM,
	config *config.File,
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
		config:          config,
		logger:          logger,
		tools:           tools,
		messages:        messages,
		messageCallback: messageCallback,
	}
}

// RunConversation executes the AI chat loop with Tool-call handling until
// the assistant produces a final answer (no further Tool calls) or an error occurs.
//
//nolint:gocyclo,funlen
func (e *Engine) RunConversation(
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
			// Handle content streaming (collect until we see a newline or done)
			if s := resp.Message.Content; s != "" {
				thisLine.WriteString(s)
				if strings.Contains(s, "\n") || resp.Done {
					if err := updater.AddContent(ctx, thisLine.String()); err != nil {
						return &StreamingUpdateError{
							Component: "Engine.AddContent",
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
			}

			// Handle Tool calls
			if len(resp.Message.ToolCalls) > 0 {
				pendingCalls = append(pendingCalls, resp.Message.ToolCalls...)
				// Log Tool calls for debugging
				e.logger.Debug("", "Engine", fmt.Sprintf("Tool call detected: %s", resp.Message.ToolCalls[0].Function.Name))
				for _, toolCall := range resp.Message.ToolCalls {
					if err := updater.AddToolCall(ctx, toolCall); err != nil {
						return &StreamingUpdateError{
							Component: "Engine.AddToolCall",
							Message:   fmt.Sprintf("failed to stream Tool call for %s", toolCall.Function.Name),
							Cause:     err,
						}
					}
				}
			}

			// Update statistics in real-time
			if resp.Done {
				cumulativeStats.IsDone = true
				cumulativeStats.EvalCount = resp.EvalCount
				cumulativeStats.DoneReason = resp.DoneReason
				cumulativeStats.ResponseTokens += resp.EvalCount
				cumulativeStats.PromptTokens += resp.PromptEvalCount
				cumulativeStats.TotalTokens = cumulativeStats.PromptTokens + cumulativeStats.ResponseTokens

				// Flush any remaining thinking before sending final stats
				if thisThinking.Len() > 0 {
					if err := updater.AddThought(ctx, thisThinking.String()); err != nil {
						return &StreamingUpdateError{
							Component: "Engine.FlushThinking",
							Message:   "failed to flush thinking content",
							Cause:     err,
						}
					}
					thinkingBuffer.WriteString(thisThinking.String())
					thisThinking.Reset()
				}

				// Send real-time stats to updater
				if statsErr := updater.UpdateStats(ctx, cumulativeStats); statsErr != nil {
					return &StreamingUpdateError{
						Component: "Engine.UpdateStats",
						Message:   "failed to update statistics",
						Cause:     statsErr,
					}
				}
			}

			return nil
		})

		if err != nil {
			e.logger.Error("", "Engine", fmt.Sprintf("Error querying LLM: %v", err))
			return &LLMConnectionError{
				Message: "LLM chat request failed",
				Cause:   err,
			}
		}

		// Flush any remaining content and thinking
		if thisLine.Len() > 0 {
			if err := updater.AddContent(ctx, thisLine.String()); err != nil {
				return &StreamingUpdateError{
					Component: "Engine.FlushContent",
					Message:   "failed to flush remaining content",
					Cause:     err,
				}
			}
		}
		if thisThinking.Len() > 0 {
			if err := updater.AddThought(ctx, thisThinking.String()); err != nil {
				return &StreamingUpdateError{
					Component: "Engine.FlushThinking",
					Message:   "failed to flush thinking content",
					Cause:     err,
				}
			}
			thinkingBuffer.WriteString(thisThinking.String())
		}

		// Record the assistant turn (including Tool calls, if any)
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
				e.logger.Error("", "Engine", fmt.Sprintf("Error in message callback: %v", err))
				return &MessageCallbackError{
					Operation: "messageCallback",
					Message:   "message callback failed",
					Cause:     err,
				}
			}
		}

		// If there are no Tool calls, we are done for this turn
		if len(pendingCalls) == 0 {
			return updater.Flush(ctx)
		}

		// Execute all pending Tool calls and collect responses
		var pendingCallsErrors error
		for _, call := range pendingCalls {
			e.logger.Debug("", "Engine", fmt.Sprintf("Invoking Tool %s", call.Function.Name))
			reply, herr := e.tools.HandleCall(ctx, call)
			if herr != nil {
				e.logger.Error("", "Engine", fmt.Sprintf("Error invoking Tool %s: %v", call.Function.Name, herr))
				wrappedErr := &ToolInvocationError{
					ToolName: call.Function.Name,
					Message:  "Tool execution failed",
					Cause:    herr,
				}
				pendingCallsErrors = errors.Join(wrappedErr, pendingCallsErrors)
			}

			// Notify updater of Tool result
			if err := updater.AddToolResult(ctx, call, reply, herr); err != nil {
				wrappedErr := &StreamingUpdateError{
					Component: "Engine.AddToolResult",
					Message:   fmt.Sprintf("failed to stream Tool result for %s", call.Function.Name),
					Cause:     err,
				}
				pendingCallsErrors = errors.Join(wrappedErr, pendingCallsErrors)
			}

			e.messages = append(e.messages, reply...)
			e.logger.Debug("", "Engine", fmt.Sprintf("Invoked Tool %s, received the following response:\n%#v\n", call.Function.Name, reply))
		}
		if pendingCallsErrors != nil {
			return pendingCallsErrors
		}

		// No Tool calls means the assistant provided a final answer - conversation is complete
		if len(pendingCalls) == 0 {
			break
		}
		// Loop continues: the next iteration sends messages including Tool outputs
	}
	// Conversation completed - flush final stats
	return updater.Flush(ctx)
}
