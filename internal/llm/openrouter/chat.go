package openrouter

import (
	"context"
	"fmt"
	"io"

	"github.com/meschbach/marvin/internal/junk"
	"github.com/meschbach/marvin/internal/llm"
	"github.com/revrost/go-openrouter"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// usage capture the token allocations while attempting to chat
type usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func (o *LLM) Chat(ctx context.Context, req *llm.ChatRequest, onResponse func(ctx context.Context, resp *llm.ChatResponse) error) error {
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
	setToolSpanAttributes(span, req.Tools)

	stream, err := o.executeWithRetry(ctx, func(ctx context.Context) (*openrouter.ChatCompletionStream, error) {
		return o.httpClient.CreateChatCompletionStream(ctx, *openRouterReq)
	})
	if err != nil {
		recordStreamError(span, err)
		return fmt.Errorf("failed to create stream: %w", err)
	}
	if stream == nil {
		return fmt.Errorf("failed to create stream: no stream returned")
	}
	defer stream.Close()

	finalUsage, finishReason, responseToolCalls, err := o.processStream(ctx, span, stream, onResponse)
	if err != nil {
		span.RecordError(err)
	}

	setStreamResultAttributes(span, finalUsage, finishReason, responseToolCalls)

	return err
}

func (o *LLM) buildRequest(ctx context.Context, req *llm.ChatRequest) (*openrouter.ChatCompletionRequest, error) {
	messages := make([]openrouter.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = o.convertMessage(ctx, &msg)
	}

	openRouterReq := &openrouter.ChatCompletionRequest{
		Model:    o.model,
		Messages: messages,
		Stream:   true,
	}

	if len(req.Tools) > 0 {
		openRouterReq.Tools = o.convertTools(req.Tools)
	}

	if req.Temperature != nil {
		openRouterReq.Temperature = *req.Temperature
	}
	if req.TopP != nil {
		openRouterReq.TopP = *req.TopP
	}
	if req.TopK != nil {
		openRouterReq.TopK = *req.TopK
	}

	return openRouterReq, nil
}

func (o *LLM) convertMessage(ctx context.Context, msg *llm.Message) openrouter.ChatCompletionMessage {
	content := msg.Content
	if msg.Role == llm.RoleAssistant && content == "" {
		content = "Thinking..."
	}
	openMsg := openrouter.ChatCompletionMessage{
		Role:    msg.Role,
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

func (o *LLM) convertTools(tools []llm.ToolDefinition) []openrouter.Tool {
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

func (o *LLM) processStream(ctx context.Context, span trace.Span, stream *openrouter.ChatCompletionStream, onResponse func(ctx context.Context, resp *llm.ChatResponse) error) (usage, openrouter.FinishReason, []string, error) {
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

		if err := o.processChunk(ctx, span, &resp, &finalUsage, onResponse, &responseToolCalls); err != nil {
			return finalUsage, finishReason, responseToolCalls, err
		}

		if o.shouldStop(&resp, finalUsage) {
			break
		}
	}

	return finalUsage, finishReason, responseToolCalls, nil
}

func (o *LLM) processChunk(ctx context.Context, span trace.Span, resp *openrouter.ChatCompletionStreamResponse, finalUsage *usage, onResponse func(ctx context.Context, resp *llm.ChatResponse) error, responseToolCalls *[]string) error {
	o.updateUsage(resp, finalUsage)

	isUsageOnly := o.isUsageOnlyChunk(resp)

	if isUsageOnly {
		return o.sendUsageOnlyChunk(ctx, resp, finalUsage, onResponse)
	}

	return o.sendContentChunk(ctx, span, resp, finalUsage, onResponse, responseToolCalls)
}

func (o *LLM) updateUsage(resp *openrouter.ChatCompletionStreamResponse, finalUsage *usage) {
	if resp.Usage != nil {
		*finalUsage = usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}
}

func (o *LLM) isUsageOnlyChunk(resp *openrouter.ChatCompletionStreamResponse) bool {
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

func (o *LLM) sendUsageOnlyChunk(ctx context.Context, resp *openrouter.ChatCompletionStreamResponse, finalUsage *usage, onResponse func(ctx context.Context, resp *llm.ChatResponse) error) error {
	if resp.Usage != nil && (resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0) {
		*finalUsage = usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
		llmResp := llm.ChatResponse{
			Done: true,
			Stats: llm.Stats{
				PromptTokens:   finalUsage.PromptTokens,
				ResponseTokens: finalUsage.CompletionTokens,
				TotalTokens:    finalUsage.TotalTokens,
			},
		}
		return onResponse(ctx, &llmResp)
	}
	return nil
}

func (o *LLM) sendContentChunk(ctx context.Context, span trace.Span, resp *openrouter.ChatCompletionStreamResponse, finalUsage *usage, onResponse func(ctx context.Context, resp *llm.ChatResponse) error, responseToolCalls *[]string) error {
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
	llmResp := llm.ChatResponse{
		Content:  delta.Content,
		Thinking: reasoning,
		Done:     isDone,
		Stats: llm.Stats{
			PromptTokens:   finalUsage.PromptTokens,
			ResponseTokens: finalUsage.CompletionTokens,
			TotalTokens:    finalUsage.TotalTokens,
		},
	}

	if delta.ToolCalls != nil {
		llmResp.ToolCalls = o.convertToolCallsFromOpenRouter(ctx, delta.ToolCalls)
		for _, tc := range delta.ToolCalls {
			*responseToolCalls = append(*responseToolCalls, tc.Function.Name)
		}
	}

	return onResponse(ctx, &llmResp)
}

func (o *LLM) isDone(finishReason openrouter.FinishReason, finalUsage usage) bool {
	return string(finishReason) != "" && (finalUsage.CompletionTokens > 0 || finalUsage.PromptTokens > 0)
}

func (o *LLM) shouldStop(resp *openrouter.ChatCompletionStreamResponse, finalUsage usage) bool {
	if len(resp.Choices) == 0 {
		return false
	}

	if resp.Choices[0].FinishReason == "" {
		return false
	}

	return finalUsage.CompletionTokens > 0 || finalUsage.PromptTokens > 0
}
