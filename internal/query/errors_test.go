package query

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLLMConnectionError(t *testing.T) {
	t.Run("Error message formatting", func(t *testing.T) {
		err := &LLMConnectionError{
			Message: "connection refused",
			Cause:   errors.New("network error"),
		}
		assert.Equal(t, "LLM connection failed: connection refused", err.Error())
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		cause := errors.New("network error")
		err := &LLMConnectionError{
			Message: "connection refused",
			Cause:   cause,
		}
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("Is matches same type", func(t *testing.T) {
		err := &LLMConnectionError{Message: "test"}
		assert.True(t, errors.Is(err, &LLMConnectionError{}))
	})

	t.Run("Is does not match different type", func(t *testing.T) {
		err := &LLMConnectionError{Message: "test"}
		assert.False(t, errors.Is(err, &ToolInvocationError{}))
		assert.False(t, errors.Is(err, &MessageCallbackError{}))
		assert.False(t, errors.Is(err, &ConfigurationError{}))
		assert.False(t, errors.Is(err, &ContextCancellationError{}))
		assert.False(t, errors.Is(err, &StreamingUpdateError{}))
	})

	t.Run("Is works with wrapped errors", func(t *testing.T) {
		inner := &LLMConnectionError{Message: "inner"}
		wrapped := &LLMConnectionError{Message: "outer", Cause: inner}
		assert.True(t, errors.Is(wrapped, &LLMConnectionError{}))
	})
}

func TestToolInvocationError(t *testing.T) {
	t.Run("Error message formatting", func(t *testing.T) {
		err := &ToolInvocationError{
			ToolName: "calculator",
			Message:  "division by zero",
			Cause:    errors.New("runtime error"),
		}
		assert.Equal(t, "Tool invocation failed (calculator): division by zero", err.Error())
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		cause := errors.New("runtime error")
		err := &ToolInvocationError{
			ToolName: "test-tool",
			Cause:    cause,
		}
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("Is matches same type", func(t *testing.T) {
		err := &ToolInvocationError{ToolName: "test"}
		assert.True(t, errors.Is(err, &ToolInvocationError{}))
	})

	t.Run("Is does not match different type", func(t *testing.T) {
		err := &ToolInvocationError{ToolName: "test"}
		assert.False(t, errors.Is(err, &LLMConnectionError{}))
	})
}

func TestMessageCallbackError(t *testing.T) {
	t.Run("Error message formatting", func(t *testing.T) {
		err := &MessageCallbackError{
			Operation: "persist",
			Message:   "database unavailable",
			Cause:     errors.New("connection timeout"),
		}
		assert.Equal(t, "Message callback failed (persist): database unavailable", err.Error())
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		cause := errors.New("connection timeout")
		err := &MessageCallbackError{
			Operation: "save",
			Cause:     cause,
		}
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("Is matches same type", func(t *testing.T) {
		err := &MessageCallbackError{Operation: "test"}
		assert.True(t, errors.Is(err, &MessageCallbackError{}))
	})

	t.Run("Is does not match different type", func(t *testing.T) {
		err := &MessageCallbackError{Operation: "test"}
		assert.False(t, errors.Is(err, &LLMConnectionError{}))
	})
}

func TestConfigurationError(t *testing.T) {
	t.Run("Error message formatting", func(t *testing.T) {
		err := &ConfigurationError{
			Component: "model-options",
			Message:   "invalid temperature value",
			Cause:     errors.New("out of range"),
		}
		assert.Equal(t, "Configuration error (model-options): invalid temperature value", err.Error())
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		cause := errors.New("file not found")
		err := &ConfigurationError{
			Component: "config-file",
			Cause:     cause,
		}
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("Is matches same type", func(t *testing.T) {
		err := &ConfigurationError{Component: "test"}
		assert.True(t, errors.Is(err, &ConfigurationError{}))
	})

	t.Run("Is does not match different type", func(t *testing.T) {
		err := &ConfigurationError{Component: "test"}
		assert.False(t, errors.Is(err, &LLMConnectionError{}))
	})
}

func TestContextCancellationError(t *testing.T) {
	t.Run("Error message formatting", func(t *testing.T) {
		err := &ContextCancellationError{
			Message: "user interrupted",
		}
		assert.Equal(t, "Context cancelled: user interrupted", err.Error())
	})

	t.Run("Unwrap returns nil", func(t *testing.T) {
		err := &ContextCancellationError{Message: "test"}
		assert.Nil(t, err.Unwrap())
	})

	t.Run("Is matches same type", func(t *testing.T) {
		err := &ContextCancellationError{Message: "test"}
		assert.True(t, errors.Is(err, &ContextCancellationError{}))
	})

	t.Run("Is does not match different type", func(t *testing.T) {
		err := &ContextCancellationError{Message: "test"}
		assert.False(t, errors.Is(err, &LLMConnectionError{}))
	})
}

func TestStreamingUpdateError(t *testing.T) {
	t.Run("Error message formatting", func(t *testing.T) {
		err := &StreamingUpdateError{
			Component: "CLIStreamingUpdater",
			Message:   "write failed",
			Cause:     errors.New("broken pipe"),
		}
		assert.Equal(t, "Streaming update failed (CLIStreamingUpdater): write failed because broken pipe", err.Error())
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		cause := errors.New("broken pipe")
		err := &StreamingUpdateError{
			Component: "SlackUpdater",
			Cause:     cause,
		}
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("Is matches same type", func(t *testing.T) {
		err := &StreamingUpdateError{Component: "test"}
		assert.True(t, errors.Is(err, &StreamingUpdateError{}))
	})

	t.Run("Is does not match different type", func(t *testing.T) {
		err := &StreamingUpdateError{Component: "test"}
		assert.False(t, errors.Is(err, &LLMConnectionError{}))
	})
}

func TestErrorTypeUniqueness(t *testing.T) {
	// Ensure all error types are distinct and don't match each other
	errorTypes := []error{
		&LLMConnectionError{},
		&ToolInvocationError{},
		&MessageCallbackError{},
		&ConfigurationError{},
		&ContextCancellationError{},
		&StreamingUpdateError{},
	}

	for i, err1 := range errorTypes {
		for j, err2 := range errorTypes {
			if i == j {
				assert.True(t, errors.Is(err1, err2), "Same error type should match itself")
			} else {
				assert.False(t, errors.Is(err1, err2), "Different error types should not match")
			}
		}
	}
}
