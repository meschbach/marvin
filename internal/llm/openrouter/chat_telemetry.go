// Telemetry helpers for OpenRouter chat operations.
// These functions capture observability data into OpenTelemetry spans without
// adding cyclomatic complexity to the core orchestration logic.
package openrouter

import (
	"errors"
	"fmt"

	"github.com/meschbach/marvin/internal/llm"
	"github.com/revrost/go-openrouter"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// setToolSpanAttributes records tool definitions as span attributes.
func setToolSpanAttributes(span trace.Span, tools []llm.ToolDefinition) {
	if len(tools) == 0 {
		return
	}
	toolNames := make([]string, len(tools))
	for i, t := range tools {
		toolNames[i] = t.Function.Name
	}
	span.SetAttributes(attribute.StringSlice("tools", toolNames))
}

// recordStreamError annotates a span with details from a stream creation failure.
// It extracts APIError metadata when available, including provider-specific errors.
func recordStreamError(span trace.Span, err error) {
	span.SetStatus(codes.Error, "chat streaming returned error.")
	span.RecordError(err)

	var apiErr *openrouter.APIError
	if !errors.As(err, &apiErr) {
		return
	}
	span.SetAttributes(
		attribute.String("error.message", apiErr.Message),
		attribute.String("error.code", fmt.Sprintf("%v", apiErr.Code)),
		attribute.Int("error.http_status", apiErr.HTTPStatusCode),
	)
	if apiErr.ProviderError != nil {
		span.SetAttributes(attribute.String("error.provider", fmt.Sprintf("%v", apiErr.ProviderError.Message())))
	}
}

// setStreamResultAttributes records final token usage, finish reason, and tool
// call information as span attributes after stream processing completes.
func setStreamResultAttributes(span trace.Span, finalUsage usage, finishReason openrouter.FinishReason, responseToolCalls []string) {
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
}
