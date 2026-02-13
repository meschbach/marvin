package conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/ollama/ollama/api"
)

type TurnResult struct {
	AssistantMessage api.Message
	PendingCalls     []api.ToolCall
	Stats            ConversationStats
}

type responseHandlerState struct {
	assistantOut    strings.Builder
	thinkingBuffer  strings.Builder
	thisLine        strings.Builder
	thisThinking    strings.Builder
	pendingCalls    []api.ToolCall
	cumulativeStats ConversationStats
}

func (e *Engine) buildChatRequest(model string) *api.ChatRequest {
	stream := true
	return &api.ChatRequest{
		Model:    model,
		Messages: e.messages,
		Tools:    e.tools.APITools(),
		Stream:   &stream,
		Options:  e.config.BuildAPIOptions(),
	}
}

func (e *Engine) handleContent(state *responseHandlerState, resp api.ChatResponse, updater StreamingUpdater) error {
	if s := resp.Message.Content; s != "" {
		state.thisLine.WriteString(s)
		if strings.Contains(s, "\n") || resp.Done {
			if err := updater.AddContent(context.Background(), state.thisLine.String()); err != nil {
				return &StreamingUpdateError{
					Component: "Engine.AddContent",
					Message:   "failed to stream content",
					Cause:     err,
				}
			}
			state.thisLine.Reset()
		}
		state.assistantOut.WriteString(s)
	}
	return nil
}

func (e *Engine) handleThinking(state *responseHandlerState, resp api.ChatResponse) {
	if len(resp.Message.Thinking) > 0 {
		state.thisThinking.WriteString(resp.Message.Thinking)
	}
}

func (e *Engine) handleToolCalls(state *responseHandlerState, resp api.ChatResponse, updater StreamingUpdater) error {
	if len(resp.Message.ToolCalls) > 0 {
		state.pendingCalls = append(state.pendingCalls, resp.Message.ToolCalls...)
		e.logger.Debug("", "Engine", fmt.Sprintf("Tool call detected: %s", resp.Message.ToolCalls[0].Function.Name))
		for _, toolCall := range resp.Message.ToolCalls {
			if err := updater.AddToolCall(context.Background(), toolCall); err != nil {
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

func (e *Engine) handleDone(state *responseHandlerState, resp api.ChatResponse, updater StreamingUpdater) error {
	state.cumulativeStats.IsDone = true
	state.cumulativeStats.EvalCount = resp.EvalCount
	state.cumulativeStats.DoneReason = resp.DoneReason
	state.cumulativeStats.ResponseTokens += resp.EvalCount
	state.cumulativeStats.PromptTokens += resp.PromptEvalCount
	state.cumulativeStats.TotalTokens = state.cumulativeStats.PromptTokens + state.cumulativeStats.ResponseTokens

	if state.thisThinking.Len() > 0 {
		if err := updater.AddThought(context.Background(), state.thisThinking.String()); err != nil {
			return &StreamingUpdateError{
				Component: "Engine.FlushThinking",
				Message:   "failed to flush thinking content",
				Cause:     err,
			}
		}
		state.thinkingBuffer.WriteString(state.thisThinking.String())
		state.thisThinking.Reset()
	}

	if statsErr := updater.UpdateStats(context.Background(), state.cumulativeStats); statsErr != nil {
		return &StreamingUpdateError{
			Component: "Engine.UpdateStats",
			Message:   "failed to update statistics",
			Cause:     statsErr,
		}
	}
	return nil
}

func (e *Engine) handleStreamingResponse(updater StreamingUpdater) (*responseHandlerState, api.ChatResponseFunc) {
	state := &responseHandlerState{}
	fn := func(resp api.ChatResponse) error {
		if err := e.handleContent(state, resp, updater); err != nil {
			return err
		}

		e.handleThinking(state, resp)

		if err := e.handleToolCalls(state, resp, updater); err != nil {
			return err
		}

		if resp.Done {
			if err := e.handleDone(state, resp, updater); err != nil {
				return err
			}
		}

		return nil
	}
	return state, fn
}

func (e *Engine) executeTurn(ctx context.Context, model string, updater StreamingUpdater) (*TurnResult, error) {
	req := e.buildChatRequest(model)

	state, responseFn := e.handleStreamingResponse(updater)

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
