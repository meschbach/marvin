package slacker

import (
	"context"
	"sync"

	"github.com/ollama/ollama/api"
	"github.com/slack-go/slack"
)

// MockLLM simulates an LLM that can be configured to return specific responses
// and track the requests it receives for verification
type MockLLM struct {
	responses [][]api.ChatResponse // Multiple response sets for multi-turn conversations
	calls     []*api.ChatRequest   // Track all calls made to the LLM
	callCount int
}

func (m *MockLLM) Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	m.calls = append(m.calls, req)

	if m.callCount >= len(m.responses) {
		return nil // No more responses configured
	}

	responses := m.responses[m.callCount]
	m.callCount++

	for _, resp := range responses {
		if err := fn(resp); err != nil {
			return err
		}
	}

	return nil
}

// Calls returns the list of requests made to the mock LLM
func (m *MockLLM) Calls() []*api.ChatRequest {
	return m.calls
}

// Reset resets the mock LLM state
func (m *MockLLM) Reset() {
	m.calls = nil
	m.callCount = 0
}

// MockSlackSink implements SlackSink for testing
type MockSlackSink struct {
	state           sync.Mutex
	PostedMessages  []string
	UpdatedMessages []string
}

func (m *MockSlackSink) UpdateMessageContext(ctx context.Context, channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
	m.state.Lock()
	defer m.state.Unlock()

	// For testing purposes, we'll capture the fact that an update was attempted
	// The actual message content testing will be done through other means
	m.UpdatedMessages = append(m.UpdatedMessages, "updated")
	return "channel", "timestamp", "text", nil
}

func (m *MockSlackSink) PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error) {
	m.state.Lock()
	defer m.state.Unlock()

	// For testing purposes, we'll capture the fact that a post was attempted
	// The actual message content testing will be done through other means
	m.PostedMessages = append(m.PostedMessages, "posted")
	return "channel", "test-timestamp", nil
}

// MockUserInterface captures user interface updates for testing
type MockUserInterface struct {
	Thoughts  []string
	Content   []string
	ToolCalls []string
}

func (c *MockUserInterface) AddThought(ctx context.Context, thought string) error {
	c.Thoughts = append(c.Thoughts, thought)
	return nil
}

func (c *MockUserInterface) AddContent(ctx context.Context, message string) error {
	c.Content = append(c.Content, message)
	return nil
}

func (c *MockUserInterface) AddToolCall(ctx context.Context, toolCall api.ToolCall) error {
	c.ToolCalls = append(c.ToolCalls, toolCall.Function.Name)
	return nil
}

// Reset clears all captured data
func (c *MockUserInterface) Reset() {
	c.Thoughts = nil
	c.Content = nil
	c.ToolCalls = nil
}
