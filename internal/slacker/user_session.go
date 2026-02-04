package slacker

import (
	"fmt"
	"time"

	"github.com/meschbach/marvin/internal/query"
	"github.com/ollama/ollama/api"
)

// UserSession represents a user's conversation session
type UserSession struct {
	UserID         string             `json:"user_id"`
	ChannelID      string             `json:"channel_id"`
	ThreadTS       string             `json:"thread_ts,omitempty"`
	LastActivity   time.Time          `json:"last_activity"`
	Messages       []api.Message      `json:"messages"`
	AvailableTools []string           `json:"available_tools"`
	ToolNamespace  string             `json:"tool_namespace"`
	UserContext    *query.UserContext `json:"-"` // Not serialized
}

// NewUserSession creates a new user session
func NewUserSession(userID, channelID string, userContext *query.UserContext) *UserSession {
	return &UserSession{
		UserID:         userID,
		ChannelID:      channelID,
		LastActivity:   time.Now(),
		Messages:       []api.Message{},
		AvailableTools: []string{},
		ToolNamespace:  fmt.Sprintf("user-%s", userID),
		UserContext:    userContext,
	}
}

// GetAvailableTools returns the list of available tools for this session
func (us *UserSession) GetAvailableTools() []string {
	// Return a copy to prevent modification
	tools := make([]string, len(us.AvailableTools))
	copy(tools, us.AvailableTools)
	return tools
}

// AddMessage adds a message to the session
func (us *UserSession) AddMessage(message api.Message) {
	us.Messages = append(us.Messages, message)
	us.LastActivity = time.Now()
}

// SetThreadTS sets the thread timestamp for the session
func (us *UserSession) SetThreadTS(threadTS string) {
	us.ThreadTS = threadTS
}

// UpdateActivity updates the last activity timestamp
func (us *UserSession) UpdateActivity() {
	us.LastActivity = time.Now()
}

// UpdateUserContext updates the user context for this session
func (us *UserSession) UpdateUserContext(userContext *query.UserContext) {
	us.UserContext = userContext
}

// SetAvailableTools sets the list of available tools for this session
func (us *UserSession) SetAvailableTools(tools []string) {
	us.AvailableTools = make([]string, len(tools))
	copy(us.AvailableTools, tools)
}

// GetRecentMessages returns the most recent messages from the session
func (us *UserSession) GetRecentMessages(limit int) []api.Message {
	if limit <= 0 {
		return []api.Message{}
	}
	if limit >= len(us.Messages) {
		return us.Messages
	}

	start := len(us.Messages) - limit
	return us.Messages[start:]
}

// GetSessionAge returns the age of the session
func (us *UserSession) GetSessionAge() time.Duration {
	return time.Since(us.LastActivity)
}

// IsExpired checks if the session has expired based on the given duration
func (us *UserSession) IsExpired(maxAge time.Duration) bool {
	return us.GetSessionAge() > maxAge
}

// ClearMessages clears all messages from the session
func (us *UserSession) ClearMessages() {
	us.Messages = []api.Message{}
}
