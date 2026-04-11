package openrouter

import (
	"context"
	"fmt"
	"io"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/junk"
	"github.com/ollama/ollama/api"
	"github.com/revrost/go-openrouter"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// usage capture the token allocations while attempting to chat
type usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func (o *LLM) Chat(ctx context.Context, req *api.ChatRequest, onEvent conversation.ChatResponseListener) error {
	ctx, span := tracer.Start(ctx, "openrouter.chat",
		trace.WithAttributes(
			attribute.String("model", o.model),
			attribute.String("url", o.baseURL),
		))
	defer span.End()

	openRouterReq, err := o.buildRequest(ctx, req)
	if err != nil {
		span.RecordError(err)
		return err
	}

	span.SetAttributes(attribute.Int("messages", len(req.Messages)))
	if len(req.Tools) > 0 {
		toolNames := make([]string, len(req.Tools))
		for i, t := range req.Tools {
			toolNames[i] = t.Function.Name
		}
		span.SetAttributes(attribute.StringSlice("tools", toolNames))
	}

	stream, err := o.executeWithRetry(ctx, func(ctx context.Context) (*openrouter.ChatCompletionStream, error) {
		return o.httpClient.CreateChatCompletionStream(ctx, *openRouterReq)
	})
	if err != nil {
		span.SetStatus(codes.Error, "chat streaming returned error.")
		span.RecordError(err)
		if apiErr, ok := err.(*openrouter.APIError); ok {
			span.SetAttributes(
				attribute.String("error.message", apiErr.Message),
				attribute.String("error.code", fmt.Sprintf("%v", apiErr.Code)),
				attribute.Int("error.http_status", apiErr.HTTPStatusCode),
			)
			if apiErr.ProviderError != nil {
				span.SetAttributes(attribute.String("error.provider", fmt.Sprintf("%v", apiErr.ProviderError.Message())))
			}
		}
		return fmt.Errorf("failed to create stream: %w", err)
	}
	if stream == nil {
		return fmt.Errorf("failed to create stream: no stream returned")
	}
	defer stream.Close()

	finalUsage, finishReason, responseToolCalls, err := o.processStream(ctx, span, stream, onEvent)
	if err != nil {
		span.RecordError(err)
	}

	span.SetAttributes(
		attribute.Int("tokens.prompt", finalUsage.PromptTokens),
		attribute.Int("tokens.completion", finalUsage.CompletionTokens),
		attribute.Int("tokens.total", finalUsage.TotalTokens),
	)
	if finishReason != "" {
		span.SetAttributes(attribute.String("finish_reason", string(finishReason)))
	}
	if len(responseToolCalls) > 0 {
		span.SetAttributes(attribute.StringSlice("tool_calls", responseToolCalls))
	}

	return err
}

func (o *LLM) buildRequest(ctx context.Context, req *api.ChatRequest) (*openrouter.ChatCompletionRequest, error) {
	messages := make([]openrouter.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = o.convertMessage(ctx, msg)
	}

	openRouterReq := &openrouter.ChatCompletionRequest{
		Model:    o.model,
		Messages: messages,
		Stream:   true,
	}

	if len(req.Tools) > 0 {
		openRouterReq.Tools = o.convertTools(req.Tools)
	}

	if req.Options != nil {
		openRouterReq.Temperature = o.extractFloat32(req.Options, "temperature")
		openRouterReq.TopP = o.extractFloat32(req.Options, "top_p")
		openRouterReq.TopK = o.extractInt(req.Options, "top_k")
		openRouterReq.MaxTokens = o.extractInt(req.Options, "num_predict")
		openRouterReq.Seed = o.extractIntPtr(req.Options, "seed")
		openRouterReq.Stop = o.extractStrings(req.Options, "stop")
	}

	return openRouterReq, nil
}

func (o *LLM) convertMessage(ctx context.Context, msg api.Message) openrouter.ChatCompletionMessage {
	content := msg.Content
	if msg.Role == conversation.RoleAssistant && content == "" {
		content = "Thinking..."
	}
	openMsg := openrouter.ChatCompletionMessage{
		Role:    string(msg.Role),
		Content: openrouter.Content{Text: content},
	}
	if msg.ToolCallID != "" {
		openMsg.ToolCallID = msg.ToolCallID
	}
	if len(msg.ToolCalls) > 0 {
		openMsg.ToolCalls = o.convertToolCallsFromOllama(ctx, msg.ToolCalls)
	}
	return openMsg
}

func (o *LLM) convertTools(tools api.Tools) []openrouter.Tool {
	result := make([]openrouter.Tool, len(tools))
	for i, t := range tools {
		result[i] = openrouter.Tool{
			Type: openrouter.ToolType(t.Type),
			Function: &openrouter.FunctionDefinition{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		}
	}
	return result
}

func (o *LLM) extractFloat32(opts map[string]any, key string) float32 {
	if val, ok := opts[key]; ok {
		if f, ok := val.(float64); ok {
			return float32(f)
		}
	}
	return 0
}

func (o *LLM) extractInt(opts map[string]any, key string) int {
	if val, ok := opts[key]; ok {
		if f, ok := val.(float64); ok {
			return int(f)
		}
	}
	return 0
}

func (o *LLM) extractIntPtr(opts map[string]any, key string) *int {
	if val, ok := opts[key]; ok {
		if f, ok := val.(float64); ok {
			i := int(f)
			return &i
		}
	}
	return nil
}

func (o *LLM) extractStrings(opts map[string]any, key string) []string {
	if val, ok := opts[key]; ok {
		if slice, ok := val.([]any); ok {
			strs := make([]string, len(slice))
			for i, v := range slice {
				if s, ok := v.(string); ok {
					strs[i] = s
				}
			}
			return strs
		}
	}
	return nil
}

func (o *LLM) processStream(ctx context.Context, span trace.Span, stream *openrouter.ChatCompletionStream, fn conversation.ChatResponseListener) (usage, openrouter.FinishReason, []string, error) {
	finalUsage := usage{}
	var finishReason openrouter.FinishReason
	var responseToolCalls []string

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			junk.RecordSpanErrorNoLint(span, err)
			return finalUsage, finishReason, responseToolCalls, fmt.Errorf("error receiving stream: %w", err)
		}

		span.AddEvent("received_event", trace.WithAttributes(attribute.Int("choices", len(resp.Choices))))
		if len(resp.Choices) > 0 && resp.Choices[0].FinishReason != "" {
			span.AddEvent("finish")
			finishReason = resp.Choices[0].FinishReason
		}

		if err := o.processChunk(ctx, span, resp, &finalUsage, fn, &responseToolCalls); err != nil {
			return finalUsage, finishReason, responseToolCalls, err
		}

		if o.shouldStop(resp, finalUsage) {
			break
		}
	}

	return finalUsage, finishReason, responseToolCalls, nil
}

func (o *LLM) processChunk(ctx context.Context, span trace.Span, resp openrouter.ChatCompletionStreamResponse, finalUsage *usage, fn conversation.ChatResponseListener, responseToolCalls *[]string) error {
	o.updateUsage(resp, finalUsage)

	isUsageOnly := o.isUsageOnlyChunk(resp)

	if isUsageOnly {
		return o.sendUsageOnlyChunk(ctx, resp, finalUsage, fn)
	}

	return o.sendContentChunk(ctx, span, resp, finalUsage, fn, responseToolCalls)
}

func (o *LLM) updateUsage(resp openrouter.ChatCompletionStreamResponse, finalUsage *usage) {
	if resp.Usage != nil {
		*finalUsage = usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}
}

func (o *LLM) isUsageOnlyChunk(resp openrouter.ChatCompletionStreamResponse) bool {
	if len(resp.Choices) == 0 {
		return true
	}

	choice := resp.Choices[0]
	hasContent := choice.Delta.Content != ""
	hasToolCalls := len(choice.Delta.ToolCalls) > 0
	hasReasoning := choice.Delta.Reasoning != nil && *choice.Delta.Reasoning != ""
	hasFinishReason := choice.FinishReason != ""

	return !hasContent && !hasToolCalls && !hasReasoning && !hasFinishReason
}

func (o *LLM) sendUsageOnlyChunk(ctx context.Context, resp openrouter.ChatCompletionStreamResponse, finalUsage *usage, onEvent conversation.ChatResponseListener) error {
	if resp.Usage != nil && (resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0) {
		*finalUsage = usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
		apiResp := &api.ChatResponse{
			Model: resp.Model,
			Done:  true,
			Metrics: api.Metrics{
				EvalCount:       finalUsage.CompletionTokens,
				PromptEvalCount: finalUsage.PromptTokens,
			},
		}
		return onEvent.OnChatResponse(ctx, apiResp)
	}
	return nil
}

func (o *LLM) sendContentChunk(ctx context.Context, span trace.Span, resp openrouter.ChatCompletionStreamResponse, finalUsage *usage, fn conversation.ChatResponseListener, responseToolCalls *[]string) error {
	choice := resp.Choices[0]
	delta := choice.Delta
	span.AddEvent("content-chunk")

	if resp.Usage != nil && (resp.Usage.CompletionTokens > 0 || resp.Usage.PromptTokens > 0) {
		*finalUsage = usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	isDone := o.isDone(choice.FinishReason, *finalUsage)

	reasoning := ""
	if delta.Reasoning != nil {
		reasoning = *delta.Reasoning
	}
	apiResp := &api.ChatResponse{
		Model: resp.Model,
		Message: api.Message{
			Role:     delta.Role,
			Content:  delta.Content,
			Thinking: reasoning,
		},
		Done: isDone,
		Metrics: api.Metrics{
			EvalCount:       finalUsage.CompletionTokens,
			PromptEvalCount: finalUsage.PromptTokens,
		},
	}

	if delta.ToolCalls != nil {
		apiResp.Message.ToolCalls = o.convertToolCallsFromOpenRouter(ctx, delta.ToolCalls)
		for _, tc := range delta.ToolCalls {
			*responseToolCalls = append(*responseToolCalls, tc.Function.Name)
		}
	}

	return fn.OnChatResponse(ctx, apiResp)
}

func (o *LLM) isDone(finishReason openrouter.FinishReason, finalUsage usage) bool {
	return string(finishReason) != "" && (finalUsage.CompletionTokens > 0 || finalUsage.PromptTokens > 0)
}

func (o *LLM) shouldStop(resp openrouter.ChatCompletionStreamResponse, finalUsage usage) bool {
	if len(resp.Choices) == 0 {
		return false
	}

	if resp.Choices[0].FinishReason == "" {
		return false
	}

	return finalUsage.CompletionTokens > 0 || finalUsage.PromptTokens > 0
}
