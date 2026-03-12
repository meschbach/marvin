package conversation

import "fmt"

// Logger provides optional logging with null object pattern
type Logger interface {
	Debug(userID, component, message string)
	Error(userID, component, message string)
}

// NullLogger provides no-op implementation for optional logging
type NullLogger struct{}

// Debug logs a debug message (no-op).
func (n *NullLogger) Debug(_, _, _ string) {}

func (n *NullLogger) Error(_, _, _ string) {}

// VerboseLogger implements a logger that prints all messages.
type VerboseLogger struct{}

// Debug logs a debug message with user ID and component context.
func (v VerboseLogger) Debug(userID, component, message string) {
	fmt.Printf("[DEBUG] {user: %s, component: %s}: %s\n", userID, component, message)
}

func (v VerboseLogger) Error(userID, component, message string) {
	fmt.Printf("[ERROR] {user: %s, component: %s}: %s\n", userID, component, message)
}
