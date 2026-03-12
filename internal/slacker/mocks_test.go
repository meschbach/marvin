package slacker

import (
	"context"
	"sync"
	"time"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

// MockLLM simulates an LLM that can be configured to return specific responses
// and track the requests it receives for verification
type MockLLM struct {
	responses [][]api.ChatResponse // Multiple response sets for multi-turn conversations
	calls     []*api.ChatRequest   // Track all calls made to the LLM
	callCount int
}

func (m *MockLLM) Chat(ctx context.Context, req *api.ChatRequest, listener conversation.ChatResponseListener) error {
	m.calls = append(m.calls, req)

	if m.callCount >= len(m.responses) {
		return nil // No more responses configured
	}

	responses := m.responses[m.callCount]
	m.callCount++

	for i := range responses {
		if err := listener.OnChatResponse(ctx, &responses[i]); err != nil {
			return err
		}
	}

	return nil
}

// Calls return the list of requests made to the mock LLM
func (m *MockLLM) Calls() []*api.ChatRequest {
	return m.calls
}

// Reset resets the mock LLM state
func (m *MockLLM) Reset() {
	m.calls = nil
	m.callCount = 0
}

// ResetCallCount resets only the call count, preserving recorded calls.
// Useful for testing multi-turn conversations where you want to verify all calls.
func (m *MockLLM) ResetCallCount() {
	m.callCount = 0
}

// MockLLMProvider implements marvin.LLMProvider for testing
type MockLLMProvider struct {
	LLM *MockLLM
}

func (m *MockLLMProvider) ObtainLLM(_ context.Context, _ string) (conversation.LLM, error) {
	return m.LLM, nil
}

// MockSlackSink implements SlackSink and SlackClient for testing
type MockSlackSink struct {
	state                sync.Mutex
	PostedMessages       []string
	UpdatedMessages      []string
	postedMessageDetails []PostedMessage // detailed captures
	authTestResponse     slack.AuthTestResponse
}

// PostedMessage captures details of a posted Slack message
type PostedMessage struct {
	ChannelID string
	Timestamp string
}

func (m *MockSlackSink) UpdateMessageContext(_ context.Context, _, _ string, _ ...slack.MsgOption) (channel, timestamp, text string, err error) {
	m.state.Lock()
	defer m.state.Unlock()

	// For testing purposes, we'll capture the fact that an update was attempted
	m.UpdatedMessages = append(m.UpdatedMessages, "updated")
	channel = "channel"
	timestamp = "timestamp"
	text = "text"
	return
}

func (m *MockSlackSink) PostMessageContext(_ context.Context, channelID string, _ ...slack.MsgOption) (channel, timestamp string, err error) {
	m.state.Lock()
	defer m.state.Unlock()

	// Record the channel ID for verification
	m.postedMessageDetails = append(m.postedMessageDetails, PostedMessage{
		ChannelID: channelID,
		Timestamp: timestamp,
	})
	m.PostedMessages = append(m.PostedMessages, "posted")
	channel = channelID
	timestamp = "test-timestamp"
	return
}

// GetPostedMessages returns all posted message details (thread-safe copy)
func (m *MockSlackSink) GetPostedMessages() []PostedMessage {
	m.state.Lock()
	defer m.state.Unlock()
	// Return a copy to avoid race conditions
	result := make([]PostedMessage, len(m.postedMessageDetails))
	copy(result, m.postedMessageDetails)
	return result
}

// WaitForAnyPost blocks until at least one message is posted or timeout expires
func (m *MockSlackSink) WaitForAnyPost(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m.state.Lock()
		count := len(m.postedMessageDetails)
		m.state.Unlock()
		if count > 0 {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func (m *MockSlackSink) GetUserInfo(userID string) (*slack.User, error) {
	return &slack.User{
		ID:   userID,
		Name: "Mock User",
	}, nil
}

func (m *MockSlackSink) OpenConversationContext(_ context.Context, _ *slack.OpenConversationParameters) (channel *slack.Channel, noSuchUser, noSuchChannel bool, err error) {
	return &slack.Channel{
		GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{
				ID: "mock-channel",
			},
		},
	}, false, false, nil
}

// AuthTest implements SlackClient. Returns configured response or a default.
func (m *MockSlackSink) AuthTest() (*slack.AuthTestResponse, error) {
	if m.authTestResponse.UserID != "" {
		return &m.authTestResponse, nil
	}
	return &slack.AuthTestResponse{
		UserID: "U123456789",
		User:   "test-bot",
		TeamID: "T123",
		Team:   "test-team",
	}, nil
}

// MockUserInterface captures user interface updates for testing
type MockUserInterface struct {
	Thoughts    []string
	Content     []string
	ToolCalls   []string
	ToolResults []struct {
		ToolCall api.ToolCall
		Result   []api.Message
		Err      error
	}
}

func (c *MockUserInterface) AddThought(_ context.Context, thought string) error {
	c.Thoughts = append(c.Thoughts, thought)
	return nil
}

func (c *MockUserInterface) AddContent(_ context.Context, message string) error {
	c.Content = append(c.Content, message)
	return nil
}

func (c *MockUserInterface) AddToolCall(_ context.Context, toolCall api.ToolCall) error {
	c.ToolCalls = append(c.ToolCalls, toolCall.Function.Name)
	return nil
}

func (c *MockUserInterface) AddToolResult(_ context.Context, toolCall api.ToolCall, result []api.Message, err error) error {
	c.ToolResults = append(c.ToolResults, struct {
		ToolCall api.ToolCall
		Result   []api.Message
		Err      error
	}{
		ToolCall: toolCall,
		Result:   result,
		Err:      err,
	})
	return nil
}

func (c *MockUserInterface) UpdateStats(_ context.Context, _ conversation.Stats) error {
	return nil
}

func (c *MockUserInterface) Flush(_ context.Context) error {
	return nil
}

// Reset clears all captured data
func (c *MockUserInterface) Reset() {
	c.Thoughts = nil
	c.Content = nil
	c.ToolCalls = nil
	c.ToolResults = nil
}

// noopSocketClient is a no-op implementation of SocketClient for testing.
type noopSocketClient struct{}

func (n *noopSocketClient) RunContext(_ context.Context) error { return nil }

func (n *noopSocketClient) Ack(_ *socketmode.Request, _ ...interface{}) {}
func (n *noopSocketClient) Events() <-chan socketmode.Event             { return nil }
