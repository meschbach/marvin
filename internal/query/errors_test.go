package query

import (
	"errors"
	"testing"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/stretchr/testify/assert"
)

func TestLLMConnectionError(t *testing.T) {
	t.Run("Error message formatting", func(t *testing.T) {
		err := &conversation.LLMConnectionError{
			Message: "connection refused",
			Cause:   errors.New("network error"),
		}
		assert.Equal(t, "LLM connection failed: connection refused", err.Error())
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		cause := errors.New("network error")
		err := &conversation.LLMConnectionError{
			Message: "connection refused",
			Cause:   cause,
		}
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("Is Matches same type", func(t *testing.T) {
		err := &conversation.LLMConnectionError{Message: "test"}
		assert.True(t, errors.Is(err, &conversation.LLMConnectionError{}))
	})

	t.Run("Is does not match different type", func(t *testing.T) {
		err := &conversation.LLMConnectionError{Message: "test"}
		assert.False(t, errors.Is(err, &conversation.ToolInvocationError{}))
		assert.False(t, errors.Is(err, &conversation.MessageCallbackError{}))
		assert.False(t, errors.Is(err, &ConfigurationError{}))
		assert.False(t, errors.Is(err, &ContextCancellationError{}))
		assert.False(t, errors.Is(err, &conversation.StreamingUpdateError{}))
	})

	t.Run("Is works with wrapped errors", func(t *testing.T) {
		inner := &conversation.LLMConnectionError{Message: "inner"}
		wrapped := &conversation.LLMConnectionError{Message: "outer", Cause: inner}
		assert.True(t, errors.Is(wrapped, &conversation.LLMConnectionError{}))
	})
}

func TestToolInvocationError(t *testing.T) {
	t.Run("Error message formatting", func(t *testing.T) {
		err := &conversation.ToolInvocationError{
			ToolName: "calculator",
			Message:  "division by zero",
			Cause:    errors.New("runtime error"),
		}
		assert.Equal(t, "Tool invocation failed (calculator): division by zero", err.Error())
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		cause := errors.New("runtime error")
		err := &conversation.ToolInvocationError{
			ToolName: "test-tool",
			Cause:    cause,
		}
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("Is Matches same type", func(t *testing.T) {
		err := &conversation.ToolInvocationError{ToolName: "test"}
		assert.True(t, errors.Is(err, &conversation.ToolInvocationError{}))
	})

	t.Run("Is does not match different type", func(t *testing.T) {
		err := &conversation.ToolInvocationError{ToolName: "test"}
		assert.False(t, errors.Is(err, &conversation.LLMConnectionError{}))
	})
}

func TestMessageCallbackError(t *testing.T) {
	t.Run("Error message formatting", func(t *testing.T) {
		err := &conversation.MessageCallbackError{
			Operation: "persist",
			Message:   "database unavailable",
			Cause:     errors.New("connection timeout"),
		}
		assert.Equal(t, "Message callback failed (persist): database unavailable", err.Error())
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		cause := errors.New("connection timeout")
		err := &conversation.MessageCallbackError{
			Operation: "save",
			Cause:     cause,
		}
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("Is Matches same type", func(t *testing.T) {
		err := &conversation.MessageCallbackError{Operation: "test"}
		assert.True(t, errors.Is(err, &conversation.MessageCallbackError{}))
	})

	t.Run("Is does not match different type", func(t *testing.T) {
		err := &conversation.MessageCallbackError{Operation: "test"}
		assert.False(t, errors.Is(err, &conversation.LLMConnectionError{}))
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

	t.Run("Is Matches same type", func(t *testing.T) {
		err := &ConfigurationError{Component: "test"}
		assert.True(t, errors.Is(err, &ConfigurationError{}))
	})

	t.Run("Is does not match different type", func(t *testing.T) {
		err := &ConfigurationError{Component: "test"}
		assert.False(t, errors.Is(err, &conversation.LLMConnectionError{}))
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

	t.Run("Is Matches same type", func(t *testing.T) {
		err := &ContextCancellationError{Message: "test"}
		assert.True(t, errors.Is(err, &ContextCancellationError{}))
	})

	t.Run("Is does not match different type", func(t *testing.T) {
		err := &ContextCancellationError{Message: "test"}
		assert.False(t, errors.Is(err, &conversation.LLMConnectionError{}))
	})
}

func TestStreamingUpdateError(t *testing.T) {
	t.Run("Error message formatting", func(t *testing.T) {
		err := &conversation.StreamingUpdateError{
			Component: "CLIStreamingUpdater",
			Message:   "write failed",
			Cause:     errors.New("broken pipe"),
		}
		assert.Equal(t, "Streaming update failed (CLIStreamingUpdater): write failed because broken pipe", err.Error())
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		cause := errors.New("broken pipe")
		err := &conversation.StreamingUpdateError{
			Component: "SlackUpdater",
			Cause:     cause,
		}
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("Is Matches same type", func(t *testing.T) {
		err := &conversation.StreamingUpdateError{Component: "test"}
		assert.True(t, errors.Is(err, &conversation.StreamingUpdateError{}))
	})

	t.Run("Is does not match different type", func(t *testing.T) {
		err := &conversation.StreamingUpdateError{Component: "test"}
		assert.False(t, errors.Is(err, &conversation.LLMConnectionError{}))
	})
}

func TestErrorTypeUniqueness(t *testing.T) {
	// Ensure all error types are distinct and don't match each other
	errorTypes := []error{
		&conversation.LLMConnectionError{},
		&conversation.ToolInvocationError{},
		&conversation.MessageCallbackError{},
		&ConfigurationError{},
		&ContextCancellationError{},
		&conversation.StreamingUpdateError{},
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
