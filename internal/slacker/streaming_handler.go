package slacker

import (
	"strings"

	"github.com/ollama/ollama/api"
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
}

// newStreamingResponseHandler creates a handler for processing streaming responses.
func newStreamingResponseHandler[T userInterface](updater T) *streamingResponseHandler[T] {
	return &streamingResponseHandler[T]{
		updater: updater,
	}
}

// handleResponse processes a single streaming response chunk.
// Maintains buffer state and updates Slack with content, thoughts, and tool calls.
func (h *streamingResponseHandler[T]) handleResponse(resp api.ChatResponse) error {
	if resp.Done {
		// Response complete - would mark updater as complete
	}

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

	// Handle thinking
	if len(resp.Message.Thinking) > 0 {
		h.thinkingBuffer.WriteString(resp.Message.Thinking)
		h.updater.AddThought(resp.Message.Thinking)
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
	// Handle any remaining content
	if h.thisLine.Len() > 0 {
		line := h.thisLine.String()
		h.updater.AddContent(line)
		h.assistantContent.WriteString(line)
	}
	if h.thisThinking.Len() > 0 {
		line := h.thinkingBuffer.String()
		h.updater.AddThought(line)
		h.thinkingBuffer.WriteString(line)
	}

	return h.assistantContent.String(), h.thinkingBuffer.String(), h.pendingCalls
}
