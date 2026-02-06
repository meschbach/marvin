package slacker

import (
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
	AddThought(thought string)
	AddContent(message string)
	AddToolCall(functionName string)
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
func (h *streamingResponseHandler[T]) handleTransition(newState streamingState) {
	if h.currentState != newState {
		// Flush previous state's buffers
		switch h.currentState {
		case stateThinking:
			if h.thisThinking.Len() > 0 {
				h.updater.AddThought(h.thisThinking.String())
				h.thinkingBuffer.WriteString(h.thisThinking.String())
				h.thisThinking.Reset()
			}
		case stateContent:
			if h.thisLine.Len() > 0 {
				h.updater.AddContent(h.thisLine.String())
				h.assistantContent.WriteString(h.thisLine.String())
				h.thisLine.Reset()
			}
		}

		h.prevState = h.currentState
		h.currentState = newState
	}
}

// handleResponse processes a single streaming response chunk.
// Maintains buffer state and updates Slack with content, thoughts, and tool calls.
func (h *streamingResponseHandler[T]) handleResponse(resp api.ChatResponse) error {
	// Detect state transition
	newState := h.detectTransition(resp)
	h.handleTransition(newState)

	if resp.Done {
		// Response complete - would mark updater as complete
	}

	// Handle all response fields (maintaining backward compatibility)
	// Handle content
	if s := resp.Message.Content; s != "" {
		h.thisLine.WriteString(s)
		if strings.Contains(s, "\n") {
			nextContent := h.thisLine.String()
			h.updater.AddContent(nextContent)
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
			h.updater.AddToolCall(toolCall.Function.Name)
		}
	}

	return nil
}

// finished returns the accumulated content, thinking, and tool calls.
// Called after streaming completes to build the final assistant message.
func (h *streamingResponseHandler[T]) finished() (string, string, []api.ToolCall) {
	// Flush any remaining content and thinking
	if h.thisLine.Len() > 0 {
		line := h.thisLine.String()
		h.updater.AddContent(line)
		h.assistantContent.WriteString(line)
	}
	if h.thisThinking.Len() > 0 {
		line := h.thisThinking.String()
		h.updater.AddThought(line)
		h.thinkingBuffer.WriteString(line)
	}

	return h.assistantContent.String(), h.thinkingBuffer.String(), h.pendingCalls
}
