package query

import "fmt"

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
