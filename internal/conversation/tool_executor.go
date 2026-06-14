package conversation

import (
	"context"
	"errors"
	"fmt"

	"github.com/meschbach/marvin/internal/llm"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (e *Engine) executeToolCalls(ctx context.Context, pendingCalls []llm.ToolCall, updater StreamingUpdater) ([]llm.Message, error) {
	ctx, span := tracer.Start(ctx, "Engine.executeToolCalls")
	defer span.End()

	var pendingCallsErrors error
	for _, call := range pendingCalls {
		span.AddEvent("tool-invocation", trace.WithAttributes(attribute.String("function.name", call.Function.Name)))
		reply, herr := e.tools.HandleCall(ctx, call)
		if herr != nil {
			span.AddEvent("tool-invocation-failed", trace.WithAttributes(attribute.String("function.name", call.Function.Name), attribute.String("error", herr.Error())))
			wrappedErr := &ToolInvocationError{
				ToolName: call.Function.Name,
				Message:  "Tool execution failed",
				Cause:    herr,
			}
			pendingCallsErrors = errors.Join(wrappedErr, pendingCallsErrors)
		}

		if err := updater.AddToolResult(ctx, call, reply, herr); err != nil {
			span.AddEvent("streaming-tool-result-failed", trace.WithAttributes(attribute.String("function.name", call.Function.Name), attribute.String("error", err.Error())))
			wrappedErr := &StreamingUpdateError{
				Component: "Engine.AddToolResult",
				Message:   fmt.Sprintf("failed to stream Tool result for %s", call.Function.Name),
				Cause:     err,
			}
			pendingCallsErrors = errors.Join(wrappedErr, pendingCallsErrors)
		}

		e.messages = append(e.messages, reply...)
	}
	return e.messages, pendingCallsErrors
}
