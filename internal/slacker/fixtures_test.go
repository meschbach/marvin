package slacker

import (
	"os"
	"testing"

	"github.com/meschbach/marvin/internal/config"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/stretchr/testify/require"
)

// TestEnvironment provides common test setup
type TestEnvironment struct {
	TempDir        string
	SessionManager *SessionManager
	SecurityLogger *sec.SecurityLogger
	Config         *config.File
	MockLLM        *MockLLM
	Updater        *SlackUpdater
	MockSlackSink  *MockSlackSink
	QueryStreamer  *QueryStreamer[*MockLLM]
}

// NewTestEnvironment creates a complete test environment for QueryStreamer
func NewTestEnvironment(t *testing.T) *TestEnvironment {
	t.Helper()

	// Create temporary directory for sessions
	tempDir, err := os.MkdirTemp("", "marvin-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	// Create components
	sessionManager, err := NewSessionManager(tempDir)
	require.NoError(t, err)

	securityLogger := sec.NewSecurityLogger()
	cfg := &config.File{Model: "test-model"}

	// Create mock components
	mockLLM := &MockLLM{}
	mockSlackSink := &MockSlackSink{}

	// Create formatter
	formatter := NewSlackFormatter()

	// Create QueryStreamer
	queryStreamer := NewQueryStreamer(
		nil, // tenantToolSet not needed for these tests
		sessionManager,
		cfg,
		securityLogger,
		formatter,
		mockLLM,
	)

	// Create SlackUpdater
	updater := NewSlackUpdater(mockSlackSink, "test-channel")

	return &TestEnvironment{
		TempDir:        tempDir,
		SessionManager: sessionManager,
		SecurityLogger: securityLogger,
		Config:         cfg,
		MockLLM:        mockLLM,
		Updater:        updater,
		MockSlackSink:  mockSlackSink,
		QueryStreamer:  queryStreamer,
	}
}

// CreateTestSlackContext creates a test SlackContext
func CreateTestSlackContext(userID, channelID, message string) *SlackContext {
	return &SlackContext{
		UserID:    userID,
		ChannelID: channelID,
		Message:   message,
	}
}

// CreateTestUserSession creates a test user session
func CreateTestUserSession(userID, channelID string) *UserSession {
	return &UserSession{
		UserID:    userID,
		ChannelID: channelID,
	}
}
