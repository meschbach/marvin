package query

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

// ToolInvocationError represents tool execution failures
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

// ConfigurationError represents configuration problems
type ConfigurationError struct {
	Component string
	Message   string
	Cause     error
}

func (e *ConfigurationError) Error() string {
	return fmt.Sprintf("Configuration error (%s): %s", e.Component, e.Message)
}

func (e *ConfigurationError) Unwrap() error {
	return e.Cause
}

// Is implements the error interface for errors.Is() type checking.
// Returns true if target is also a ConfigurationError.
func (e *ConfigurationError) Is(target error) bool {
	_, ok := target.(*ConfigurationError)
	return ok
}

// ContextCancellationError represents context cancellation
type ContextCancellationError struct {
	Message string
}

func (e *ContextCancellationError) Error() string {
	return fmt.Sprintf("Context cancelled: %s", e.Message)
}

func (e *ContextCancellationError) Unwrap() error {
	return nil
}

// Is implements the error interface for errors.Is() type checking.
// Returns true if target is also a ContextCancellationError.
func (e *ContextCancellationError) Is(target error) bool {
	_, ok := target.(*ContextCancellationError)
	return ok
}

// StreamingUpdateError represents streaming updater issues
type StreamingUpdateError struct {
	Component string
	Message   string
	Cause     error
}

func (e *StreamingUpdateError) Error() string {
	return fmt.Sprintf("Streaming update failed (%s): %s", e.Component, e.Message)
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
