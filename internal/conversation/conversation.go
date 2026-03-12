// Package conversation provides core logic for managing AI conversations and tools.
package conversation

import (
	"context"

	"github.com/ollama/ollama/api"
)

// Role constants used throughout the conversation engine and related components.
// These provide centralized, well-documented role definitions.
const (
	// RoleSystem represents system messages that set context and behavior
	RoleSystem = "system"

	// RoleUser represents user input messages
	RoleUser = "user"

	// RoleAssistant represents AI-generated responses
	RoleAssistant = "assistant"

	// RoleToolResult represents Tool execution results
	RoleToolResult = "tool"
)

// MessageCallback is called when a new message should be added to the conversation.
// This enables integration with external systems like Slack's session management.
type MessageCallback func(ctx context.Context, msg api.Message) error

// ChatResponseListener receives streaming chat responses from the LLM.
type ChatResponseListener interface {
	OnChatResponse(ctx context.Context, resp *api.ChatResponse) error
}

// LLM interface abstracts the underlying LLM client for testability and future provider support
type LLM interface {
	Chat(ctx context.Context, req *api.ChatRequest, listener ChatResponseListener) error
}
