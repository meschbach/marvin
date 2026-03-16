package commands

import (
	"context"
	"strings"
	"testing"
)

// mockSecurityLogger implements SecurityLogger for testing
type mockSecurityLogger struct {
	logAdminActionCalls []struct {
		adminID string
		action  string
		target  string
	}
}

func (m *mockSecurityLogger) LogError(userID, operation, message string)          {}
func (m *mockSecurityLogger) LogSessionEvent(userID, channelID, event string)     {}
func (m *mockSecurityLogger) LogToolRequest(userID, toolType, config string)      {}
func (m *mockSecurityLogger) LogToolAdded(userID, toolID, toolType string)        {}
func (m *mockSecurityLogger) LogToolRemoved(userID, toolID string)                {}
func (m *mockSecurityLogger) LogToolShare(userID, toolID, targetWorkspace string) {}
func (m *mockSecurityLogger) LogConfigChange(userID, configType, details string)  {}
func (m *mockSecurityLogger) LogAdminAction(adminID, action, target string) {
	m.logAdminActionCalls = append(m.logAdminActionCalls, struct {
		adminID string
		action  string
		target  string
	}{adminID, action, target})
}

// mockMessageSender implements MessageSender for testing
type mockMessageSender struct {
	messages []struct {
		userID  string
		message string
	}
}

func (m *mockMessageSender) SendMessage(ctx context.Context, userID, message string) error {
	m.messages = append(m.messages, struct {
		userID  string
		message string
	}{userID, message})
	return nil
}

// mockContext implements Context for testing
type mockContext struct {
	userID    string
	userName  string
	channelID string
	teamID    string
}

func (m *mockContext) UserID() string    { return m.userID }
func (m *mockContext) UserName() string  { return m.userName }
func (m *mockContext) ChannelID() string { return m.channelID }
func (m *mockContext) TeamID() string    { return m.teamID }

func TestHandleAdminHelp_LogsAdminAction(t *testing.T) {
	t.Parallel()

	secLogger := &mockSecurityLogger{}
	msgSender := &mockMessageSender{}
	ctx := &mockContext{userID: "U123"}

	deps := &CommandsDependencies{
		Context:        ctx,
		SecurityLogger: secLogger,
		MessageSender:  msgSender,
	}

	err := HandleAdminHelp(context.Background(), deps, "")
	if err != nil {
		t.Fatalf("HandleAdminHelp returned error: %v", err)
	}

	if len(secLogger.logAdminActionCalls) != 1 {
		t.Fatalf("expected 1 LogAdminAction call, got %d", len(secLogger.logAdminActionCalls))
	}
	call := secLogger.logAdminActionCalls[0]
	if call.adminID != "U123" {
		t.Errorf("expected adminID 'U123', got %q", call.adminID)
	}
	if call.action != "admin_help" {
		t.Errorf("expected action 'admin_help', got %q", call.action)
	}
	if call.target != "" {
		t.Errorf("expected empty target, got %q", call.target)
	}
}

func TestHandleAdminHelp_SendsMessage(t *testing.T) {
	t.Parallel()

	secLogger := &mockSecurityLogger{}
	msgSender := &mockMessageSender{}
	ctx := &mockContext{userID: "U456"}

	deps := &CommandsDependencies{
		Context:        ctx,
		SecurityLogger: secLogger,
		MessageSender:  msgSender,
	}

	err := HandleAdminHelp(context.Background(), deps, "")
	if err != nil {
		t.Fatalf("HandleAdminHelp returned error: %v", err)
	}

	if len(msgSender.messages) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(msgSender.messages))
	}
	sent := msgSender.messages[0]
	if sent.userID != "U456" {
		t.Errorf("expected userID 'U456', got %q", sent.userID)
	}
	if sent.message == "" {
		t.Error("expected non-empty message")
	}
}

func TestHandleAdminHelp_MessageContent(t *testing.T) {
	t.Parallel()

	secLogger := &mockSecurityLogger{}
	msgSender := &mockMessageSender{}
	ctx := &mockContext{userID: "U789"}

	deps := &CommandsDependencies{
		Context:        ctx,
		SecurityLogger: secLogger,
		MessageSender:  msgSender,
	}

	err := HandleAdminHelp(context.Background(), deps, "")
	if err != nil {
		t.Fatalf("HandleAdminHelp returned error: %v", err)
	}

	message := msgSender.messages[0].message

	// Check for required commands as per spec
	requiredPhrases := []string{
		"list pending requests",
		"approve tool",
		"reject tool",
		"model access",
	}
	for _, phrase := range requiredPhrases {
		if !strings.Contains(strings.ToLower(message), strings.ToLower(phrase)) {
			t.Errorf("admin help message missing required phrase: %q", phrase)
		}
	}
}
