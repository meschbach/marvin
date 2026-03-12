package conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/meschbach/marvin/internal/junk"
	"github.com/ollama/ollama/api"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TurnResult contains the results of a single conversation turn
type TurnResult struct {
	AssistantMessage api.Message
	PendingCalls     []api.ToolCall
	Stats            Stats
}

type responseHandlerState struct {
	assistantOut    strings.Builder
	thinkingBuffer  strings.Builder
	thisLine        strings.Builder
	thisThinking    strings.Builder
	pendingCalls    []api.ToolCall
	cumulativeStats Stats
}

func (e *Engine) buildChatRequest(model string) *api.ChatRequest {
	stream := true
	var opts map[string]any
	if e.config != nil {
		opts = e.config.BuildAPIOptions()
	}
	return &api.ChatRequest{
		Model:    model,
		Messages: e.messages,
		Tools:    e.tools.APITools(),
		Stream:   &stream,
		Options:  opts,
	}
}

func (e *Engine) handleContent(ctx context.Context, state *responseHandlerState, resp *api.ChatResponse, updater StreamingUpdater) error {
	if s := resp.Message.Content; s != "" {
		span := trace.SpanFromContext(ctx)
		newLineOrDone := false
		state.thisLine.WriteString(s)
		if strings.Contains(s, "\n") || resp.Done {
			newLineOrDone = true
			if err := updater.AddContent(ctx, state.thisLine.String()); err != nil {
				junk.RecordSpanErrorNoLint(span, err)
				return &StreamingUpdateError{
					Component: "Engine.AddContent",
					Message:   "failed to stream content",
					Cause:     err,
				}
			}
			state.thisLine.Reset()
		}
		state.assistantOut.WriteString(s)
		span.AddEvent("turn.content", trace.WithAttributes(attribute.Bool("chat.newline_or_done", newLineOrDone), attribute.Int("chat.assistant_out", state.assistantOut.Len())))
	}
	return nil
}

func (e *Engine) handleThinking(ctx context.Context, state *responseHandlerState, resp *api.ChatResponse) {
	if resp.Message.Thinking != "" {
		state.thisThinking.WriteString(resp.Message.Thinking)
		trace.SpanFromContext(ctx).AddEvent("turn.thinking", trace.WithAttributes(attribute.Int("thinking", state.thisThinking.Len())))
	}
}

func (e *Engine) handleToolCalls(ctx context.Context, state *responseHandlerState, resp *api.ChatResponse, updater StreamingUpdater) error {
	if len(resp.Message.ToolCalls) > 0 {
		trace.SpanFromContext(ctx).AddEvent("turn.tool_call")
		state.pendingCalls = append(state.pendingCalls, resp.Message.ToolCalls...)
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
	return nil
}

func (e *Engine) handleDone(ctx context.Context, state *responseHandlerState, resp *api.ChatResponse, updater StreamingUpdater) error {
	trace.SpanFromContext(ctx).AddEvent("turn.done")
	state.cumulativeStats.IsDone = true
	state.cumulativeStats.EvalCount = resp.EvalCount
	state.cumulativeStats.DoneReason = resp.DoneReason
	state.cumulativeStats.ResponseTokens += resp.EvalCount
	state.cumulativeStats.PromptTokens += resp.PromptEvalCount
	state.cumulativeStats.TotalTokens = state.cumulativeStats.PromptTokens + state.cumulativeStats.ResponseTokens

	if state.thisThinking.Len() > 0 {
		if err := updater.AddThought(ctx, state.thisThinking.String()); err != nil {
			return &StreamingUpdateError{
				Component: "Engine.FlushThinking",
				Message:   "failed to flush thinking content",
				Cause:     err,
			}
		}
		state.thinkingBuffer.WriteString(state.thisThinking.String())
		state.thisThinking.Reset()
	}

	if statsErr := updater.UpdateStats(ctx, state.cumulativeStats); statsErr != nil {
		return &StreamingUpdateError{
			Component: "Engine.UpdateStats",
			Message:   "failed to update statistics",
			Cause:     statsErr,
		}
	}
	return nil
}

type chatResponseListener struct {
	engine  *Engine
	span    trace.Span
	state   *responseHandlerState
	updater StreamingUpdater
}

func (c *chatResponseListener) OnChatResponse(ctx context.Context, resp *api.ChatResponse) error {
	c.span.AddEvent("chat.response", trace.WithAttributes(
		attribute.Int("content", len(resp.Message.Content)),
		attribute.Int("thinking", len(resp.Message.Thinking)),
		attribute.Int("tool.count", len(resp.Message.ToolCalls)),
		attribute.Int("tool.eval_count", resp.EvalCount),
	))
	if err := c.engine.handleContent(ctx, c.state, resp, c.updater); err != nil {
		return junk.RecordSpanError(c.span, err)
	}

	c.engine.handleThinking(ctx, c.state, resp)

	if err := c.engine.handleToolCalls(ctx, c.state, resp, c.updater); err != nil {
		return junk.RecordSpanError(c.span, err)
	}

	if resp.Done {
		if err := c.engine.handleDone(ctx, c.state, resp, c.updater); err != nil {
			return junk.RecordSpanError(c.span, err)
		}
	}

	return nil
}

func (e *Engine) handleStreamingResponse(ctx context.Context, updater StreamingUpdater) (*responseHandlerState, ChatResponseListener) {
	span := trace.SpanFromContext(ctx)
	state := &responseHandlerState{}
	c := &chatResponseListener{engine: e, span: span, state: state, updater: updater}
	return state, c
}

func (e *Engine) executeTurn(ctx context.Context, model string, updater StreamingUpdater) (*TurnResult, error) {
	req := e.buildChatRequest(model)

	state, responseFn := e.handleStreamingResponse(ctx, updater)

	err := e.client.Chat(ctx, req, responseFn)
	if err != nil {
		e.logger.Error("", "Engine", fmt.Sprintf("Error querying LLM: %v", err))
		return nil, &LLMConnectionError{
			Message: "LLM chat request failed",
			Cause:   err,
		}
	}

	if state.thisLine.Len() > 0 {
		if err := updater.AddContent(ctx, state.thisLine.String()); err != nil {
			return nil, &StreamingUpdateError{
				Component: "Engine.FlushContent",
				Message:   "failed to flush remaining content",
				Cause:     err,
			}
		}
	}
	if state.thisThinking.Len() > 0 {
		if err := updater.AddThought(ctx, state.thisThinking.String()); err != nil {
			return nil, &StreamingUpdateError{
				Component: "Engine.FlushThinking",
				Message:   "failed to flush thinking content",
				Cause:     err,
			}
		}
		state.thinkingBuffer.WriteString(state.thisThinking.String())
	}

	assistantMsg := api.Message{
		Role:      RoleAssistant,
		Content:   state.assistantOut.String(),
		ToolCalls: state.pendingCalls,
		Thinking:  state.thinkingBuffer.String(),
	}
	e.messages = append(e.messages, assistantMsg)

	if e.messageCallback != nil {
		if err := e.messageCallback(ctx, assistantMsg); err != nil {
			e.logger.Error("", "Engine", fmt.Sprintf("Error in message callback: %v", err))
			return nil, &MessageCallbackError{
				Operation: "messageCallback",
				Message:   "message callback failed",
				Cause:     err,
			}
		}
	}

	return &TurnResult{
		AssistantMessage: assistantMsg,
		PendingCalls:     state.pendingCalls,
		Stats:            state.cumulativeStats,
	}, nil
}
