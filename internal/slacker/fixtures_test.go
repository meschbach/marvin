package slacker

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/meschbach/marvin/internal/config"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/slack-go/slack"
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

type capturedFormat struct {
	CType   ContentType
	Content string
}
type captureFormatter struct {
	State sync.Mutex
	Give  []slack.Block
	Given []capturedFormat
}

func (c *captureFormatter) Format(ctx context.Context, content string, contentType ContentType) ([]slack.Block, error) {
	c.State.Lock()
	defer c.State.Unlock()
	c.Given = append(c.Given, capturedFormat{CType: contentType, Content: content})
	return c.Give, nil
}

func newCaptureFormatter(give []slack.Block) *captureFormatter {
	return &captureFormatter{
		State: sync.Mutex{},
		Give:  give,
		Given: nil,
	}
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
	updater := NewSlackUpdater(mockSlackSink, "test-channel", &captureFormatter{
		State: sync.Mutex{},
		Give:  nil,
		Given: nil,
	})

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

// MockTimeProvider provides deterministic time for testing
type MockTimeProvider struct {
	CurrentTime time.Time
}

func (m *MockTimeProvider) Now() time.Time {
	return m.CurrentTime
}

func (m *MockTimeProvider) Advance(duration time.Duration) {
	m.CurrentTime = m.CurrentTime.Add(duration)
}
