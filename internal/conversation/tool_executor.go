package conversation

import (
	"context"
	"errors"
	"fmt"

	"github.com/ollama/ollama/api"
)

func (e *Engine) executeToolCalls(ctx context.Context, pendingCalls []api.ToolCall, updater StreamingUpdater) ([]api.Message, error) {
	var pendingCallsErrors error
	for _, call := range pendingCalls {
		e.logger.Debug("", "Engine", fmt.Sprintf("Invoking Tool %s", call.Function.Name))
		reply, herr := e.tools.HandleCall(ctx, call)
		if herr != nil {
			e.logger.Error("", "Engine", fmt.Sprintf("Error invoking Tool %s: %v", call.Function.Name, herr))
			wrappedErr := &ToolInvocationError{
				ToolName: call.Function.Name,
				Message:  "Tool execution failed",
				Cause:    herr,
			}
			pendingCallsErrors = errors.Join(wrappedErr, pendingCallsErrors)
		}

		if err := updater.AddToolResult(ctx, call, reply, herr); err != nil {
			wrappedErr := &StreamingUpdateError{
				Component: "Engine.AddToolResult",
				Message:   fmt.Sprintf("failed to stream Tool result for %s", call.Function.Name),
				Cause:     err,
			}
			pendingCallsErrors = errors.Join(wrappedErr, pendingCallsErrors)
		}

		e.messages = append(e.messages, reply...)
		e.logger.Debug("", "Engine", fmt.Sprintf("Invoked Tool %s, received the following response:\n%#v\n", call.Function.Name, reply))
	}
	return e.messages, pendingCallsErrors
}
