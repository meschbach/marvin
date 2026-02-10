package slacker

import (
	"context"
	"fmt"
)

// MockNotificationCall represents a captured call to the notification sender
type MockNotificationCall struct {
	UserID  string
	Message string
}

// MockNotificationSender is a mock implementation that captures calls without making Slack API calls
type MockNotificationSender struct {
	calls []MockNotificationCall
}

// NewMockNotificationSender creates a mock notification sender for testing
func NewMockNotificationSender() *MockNotificationSender {
	return &MockNotificationSender{
		calls: make([]MockNotificationCall, 0),
	}
}

// SendMessage captures the call parameters without making Slack API calls
func (m *MockNotificationSender) SendMessage(ctx context.Context, userID, message string) error {
	call := MockNotificationCall{
		UserID:  userID,
		Message: message,
	}
	m.calls = append(m.calls, call)
	return nil
}

// NotifyAdmins captures the call without making Slack API calls
func (m *MockNotificationSender) NotifyAdmins(ctx context.Context, request *ToolApprovalRequest) error {
	// For testing, we'll just return nil
	return nil
}

// SendApprovalNotification captures the call without making Slack API calls
func (m *MockNotificationSender) SendApprovalNotification(ctx context.Context, requesterID, adminID, requestID, status, toolID, reason string) error {
	call := MockNotificationCall{
		UserID:  requesterID, // Send to requester, not admin
		Message: fmt.Sprintf("Tool request %s (%s) has been %s by admin %s. Reason: %s", requestID, toolID, status, adminID, reason),
	}
	m.calls = append(m.calls, call)
	return nil
}

// GetCalls returns all captured calls for testing assertions
func (m *MockNotificationSender) GetCalls() []MockNotificationCall {
	return m.calls
}

// ClearCalls clears all captured calls
func (m *MockNotificationSender) ClearCalls() {
	m.calls = make([]MockNotificationCall, 0)
}

// HasMessage checks if a specific message was sent to a user
func (m *MockNotificationSender) HasMessage(userID, messageSubstring string) bool {
	for _, call := range m.calls {
		if call.UserID == userID && call.Message == messageSubstring {
			return true
		}
	}
	return false
}

// HasMessageContaining checks if a message containing the substring was sent to a user
func (m *MockNotificationSender) HasMessageContaining(userID, messageSubstring string) bool {
	for _, call := range m.calls {
		if call.UserID == userID && call.Message != "" {
			// Simple substring check - could be enhanced with regex if needed
			if len(call.Message) >= len(messageSubstring) {
				for i := 0; i <= len(call.Message)-len(messageSubstring); i++ {
					if call.Message[i:i+len(messageSubstring)] == messageSubstring {
						return true
					}
				}
			}
		}
	}
	return false
}
