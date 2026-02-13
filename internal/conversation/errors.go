package conversation

import "fmt"

// LLMConnectionError represents LLM provider connection issues
type LLMConnectionError struct {
	Message string
	Cause   error
}

func (e *LLMConnectionError) Error() string {
	return fmt.Sprintf("LLM connection failed: %s", e.Message)
}

func (e *LLMConnectionError) Unwrap() error {
	return e.Cause
}

// Is implements the error interface for errors.Is() type checking.
// Returns true if target is also an LLMConnectionError.
func (e *LLMConnectionError) Is(target error) bool {
	_, ok := target.(*LLMConnectionError)
	return ok
}

// ToolInvocationError represents Tool execution failures
type ToolInvocationError struct {
	ToolName string
	Message  string
	Cause    error
}

func (e *ToolInvocationError) Error() string {
	return fmt.Sprintf("Tool invocation failed (%s): %s", e.ToolName, e.Message)
}

func (e *ToolInvocationError) Unwrap() error {
	return e.Cause
}

// Is implements the error interface for errors.Is() type checking.
// Returns true if target is also a ToolInvocationError.
func (e *ToolInvocationError) Is(target error) bool {
	_, ok := target.(*ToolInvocationError)
	return ok
}

// MessageCallbackError represents session callback failures
type MessageCallbackError struct {
	Operation string
	Message   string
	Cause     error
}

func (e *MessageCallbackError) Error() string {
	return fmt.Sprintf("Message callback failed (%s): %s", e.Operation, e.Message)
}

func (e *MessageCallbackError) Unwrap() error {
	return e.Cause
}

// Is implements the error interface for errors.Is() type checking.
// Returns true if target is also a MessageCallbackError.
func (e *MessageCallbackError) Is(target error) bool {
	_, ok := target.(*MessageCallbackError)
	return ok
}

// StreamingUpdateError represents streaming updater issues
type StreamingUpdateError struct {
	Component string
	Message   string
	Cause     error
}

func (e *StreamingUpdateError) Error() string {
	return fmt.Sprintf("Streaming update failed (%s): %s because %s", e.Component, e.Message, e.Cause)
}

func (e *StreamingUpdateError) Unwrap() error {
	return e.Cause
}

// Is implements the error interface for errors.Is() type checking.
// Returns true if target is also a StreamingUpdateError.
func (e *StreamingUpdateError) Is(target error) bool {
	_, ok := target.(*StreamingUpdateError)
	return ok
}
