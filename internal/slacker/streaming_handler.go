package slacker

import (
	"context"
	"fmt"
	"strings"

	"github.com/ollama/ollama/api"
)

// streamingState represents the current state of streaming
type streamingState int

const (
	stateIdle streamingState = iota
	stateThinking
	stateContent
	stateToolCalls
	stateDone
)

type userInterface interface {
	AddThought(ctx context.Context, thought string) error
	AddContent(ctx context.Context, message string) error
	AddToolCall(ctx context.Context, functionName string) error
}

// streamingResponseHandler processes LLM streaming responses and updates Slack.
type streamingResponseHandler[T userInterface] struct {
	updater          T
	assistantContent strings.Builder
	thinkingBuffer   strings.Builder
	pendingCalls     []api.ToolCall
	thisLine         strings.Builder
	thisThinking     strings.Builder
	currentState     streamingState
	prevState        streamingState
}

// newStreamingResponseHandler creates a handler for processing streaming responses.
func newStreamingResponseHandler[T userInterface](updater T) *streamingResponseHandler[T] {
	return &streamingResponseHandler[T]{
		updater:      updater,
		currentState: stateIdle,
		prevState:    stateIdle,
	}
}

// detectTransition determines the current state based on response content
func (h *streamingResponseHandler[T]) detectTransition(resp api.ChatResponse) streamingState {
	hasContent := resp.Message.Content != ""
	hasThinking := resp.Message.Thinking != ""
	hasToolCalls := len(resp.Message.ToolCalls) > 0

	// Priority-based state determination
	if hasToolCalls {
		return stateToolCalls
	} else if hasThinking {
		return stateThinking
	} else if hasContent {
		return stateContent
	} else if resp.Done {
		return stateDone
	}

	return stateIdle
}

// handleTransition flushes buffers when state changes occur
func (h *streamingResponseHandler[T]) handleTransition(ctx context.Context, newState streamingState) error {
	if h.currentState != newState {
		// Flush previous state's buffers
		switch h.currentState {
		case stateThinking:
			if h.thisThinking.Len() > 0 {
				if err := h.updater.AddThought(ctx, h.thisThinking.String()); err != nil {
					return fmt.Errorf("failed to add thought: %w", err)
				}
				h.thinkingBuffer.WriteString(h.thisThinking.String())
				h.thisThinking.Reset()
			}
		case stateContent:
			if h.thisLine.Len() > 0 {
				if err := h.updater.AddContent(ctx, h.thisLine.String()); err != nil {
					return fmt.Errorf("failed to add content: %w", err)
				}
				h.assistantContent.WriteString(h.thisLine.String())
				h.thisLine.Reset()
			}
		}

		h.prevState = h.currentState
		h.currentState = newState
	}
	return nil
}

// handleResponse processes a single streaming response chunk.
// Maintains buffer state and updates Slack with content, thoughts, and tool calls.
func (h *streamingResponseHandler[T]) handleResponse(ctx context.Context, resp api.ChatResponse) error {
	// Detect state transition
	newState := h.detectTransition(resp)
	if err := h.handleTransition(ctx, newState); err != nil {
		return fmt.Errorf("failed to handle transition: %w", err)
	}

	if resp.Done {
		// Response complete - would mark updater as complete
	}

	// Handle all response fields (maintaining backward compatibility)
	// Handle content
	if s := resp.Message.Content; s != "" {
		h.thisLine.WriteString(s)
		if strings.Contains(s, "\n") {
			nextContent := h.thisLine.String()
			if err := h.updater.AddContent(ctx, nextContent); err != nil {
				return fmt.Errorf("failed to add content: %w", err)
			}
			h.assistantContent.WriteString(nextContent)
			h.thisLine.Reset()
		}
	}

	// Handle thinking (now flushes on transitions instead of immediately)
	if len(resp.Message.Thinking) > 0 {
		h.thisThinking.WriteString(resp.Message.Thinking)
		// Note: Don't immediately add to updater - wait for transition
	}

	// Handle tool calls
	if len(resp.Message.ToolCalls) > 0 {
		h.pendingCalls = append(h.pendingCalls, resp.Message.ToolCalls...)
		// Log tool calls in Slack update
		for _, toolCall := range resp.Message.ToolCalls {
			if err := h.updater.AddToolCall(ctx, toolCall.Function.Name); err != nil {
				return fmt.Errorf("failed to add tool call: %w", err)
			}
		}
	}

	return nil
}

// finished returns the accumulated content, thinking, and tool calls.
// Called after streaming completes to build the final assistant message.
func (h *streamingResponseHandler[T]) finished(ctx context.Context) (string, string, []api.ToolCall, error) {
	// Flush any remaining content and thinking
	if h.thisLine.Len() > 0 {
		line := h.thisLine.String()
		if err := h.updater.AddContent(ctx, line); err != nil {
			return "", "", nil, fmt.Errorf("failed to add final content: %w", err)
		}
		h.assistantContent.WriteString(line)
	}
	if h.thisThinking.Len() > 0 {
		line := h.thisThinking.String()
		if err := h.updater.AddThought(ctx, line); err != nil {
			return "", "", nil, fmt.Errorf("failed to add final thought: %w", err)
		}
		h.thinkingBuffer.WriteString(line)
	}

	return h.assistantContent.String(), h.thinkingBuffer.String(), h.pendingCalls, nil
}
