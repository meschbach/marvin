package conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/meschbach/marvin/internal/junk"
	"github.com/meschbach/marvin/internal/llm"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TurnResult contains the results of a single conversation turn
type TurnResult struct {
	AssistantMessage llm.Message
	PendingCalls     []llm.ToolCall
	Stats            Stats
}

type responseHandlerState struct {
	assistantOut    strings.Builder
	thinkingBuffer  strings.Builder
	thisLine        strings.Builder
	thisThinking    strings.Builder
	pendingCalls    []llm.ToolCall
	cumulativeStats Stats
}

func (e *Engine) buildChatRequest(model string) *llm.ChatRequest {
	req := &llm.ChatRequest{
		Model:    model,
		Messages: make([]llm.Message, len(e.messages)),
		Tools:    e.tools.APITools(),
	}
	copy(req.Messages, e.messages)

	if e.config != nil && e.config.Options != nil {
		if e.config.Options.Temperature != nil {
			req.Temperature = e.config.Options.Temperature
		}
		if e.config.Options.TopK != nil {
			req.TopK = e.config.Options.TopK
		}
		if e.config.Options.TopP != nil {
			req.TopP = e.config.Options.TopP
		}
	}

	return req
}

func (e *Engine) handleContent(ctx context.Context, state *responseHandlerState, resp *llm.ChatResponse, updater StreamingUpdater) error {
	if s := resp.Content; s != "" {
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

func (e *Engine) handleThinking(ctx context.Context, state *responseHandlerState, resp *llm.ChatResponse) {
	if resp.Thinking != "" {
		state.thisThinking.WriteString(resp.Thinking)
		trace.SpanFromContext(ctx).AddEvent("turn.thinking", trace.WithAttributes(attribute.Int("thinking", state.thisThinking.Len())))
	}
}

func (e *Engine) handleToolCalls(ctx context.Context, state *responseHandlerState, resp *llm.ChatResponse, updater StreamingUpdater) error {
	if len(resp.ToolCalls) > 0 {
		trace.SpanFromContext(ctx).AddEvent("turn.tool_call")
		state.pendingCalls = append(state.pendingCalls, resp.ToolCalls...)
		e.logger.Debug("", "Engine", fmt.Sprintf("Tool call detected: %s", resp.ToolCalls[0].Function.Name))
		for _, toolCall := range resp.ToolCalls {
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

func (e *Engine) handleDone(ctx context.Context, state *responseHandlerState, resp *llm.ChatResponse, updater StreamingUpdater) error {
	trace.SpanFromContext(ctx).AddEvent("turn.done")
	state.cumulativeStats.IsDone = true
	state.cumulativeStats.EvalCount = resp.Stats.ResponseTokens
	state.cumulativeStats.DoneReason = resp.Stats.DoneReason
	state.cumulativeStats.ResponseTokens += resp.Stats.ResponseTokens
	state.cumulativeStats.PromptTokens += resp.Stats.PromptTokens
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

func (e *Engine) executeTurn(ctx context.Context, model string, updater StreamingUpdater) (*TurnResult, error) {
	req := e.buildChatRequest(model)

	state := &responseHandlerState{}
	span := trace.SpanFromContext(ctx)

	err := e.client.Chat(ctx, req, func(ctx context.Context, resp *llm.ChatResponse) error {
		span.AddEvent("chat.response", trace.WithAttributes(
			attribute.Int("content", len(resp.Content)),
			attribute.Int("thinking", len(resp.Thinking)),
			attribute.Int("tool.count", len(resp.ToolCalls)),
			attribute.Int("tool.eval_count", resp.Stats.ResponseTokens),
		))
		if err := e.handleContent(ctx, state, resp, updater); err != nil {
			return junk.RecordSpanError(span, err)
		}

		e.handleThinking(ctx, state, resp)

		if err := e.handleToolCalls(ctx, state, resp, updater); err != nil {
			return junk.RecordSpanError(span, err)
		}

		if resp.Done {
			if err := e.handleDone(ctx, state, resp, updater); err != nil {
				return junk.RecordSpanError(span, err)
			}
		}

		return nil
	})
	if err != nil {
		e.logger.Error("", "Engine", fmt.Sprintf("Error querying LLM: %v", err))
		return nil, &LLMConnectionError{
			Message: "LLM chat request failed",
			Cause:   err,
		}
	}

	return e.finalizeTurn(ctx, state, updater)
}

func (e *Engine) finalizeTurn(ctx context.Context, state *responseHandlerState, updater StreamingUpdater) (*TurnResult, error) {
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

	assistantMsg := llm.Message{
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
