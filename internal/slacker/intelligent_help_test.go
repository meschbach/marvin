package slacker

import (
	"context"
	"testing"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/stretchr/testify/assert"
)

func TestMessageHandler_IntentFailureHelp(t *testing.T) {
	// Create a mock help analyzer
	helpAnalyzer := &HelpAnalyzer{}

	// Create message handler components for testing
	intentProcessor := &IntentProcessor{}
	toolManager := &ToolManagerImpl{}
	sessionManager := &SessionManager{}
	securityLogger := sec.NewSecurityLogger()
	config := &config.File{}
	tenantToolSet := &query.TenantToolSet{}

	// Test that MessageHandler can be created with help analyzer
	mh := NewMessageHandler(
		intentProcessor,
		nil, // Slack connection not needed for this test
		&mockQueryHandler{},
		toolManager,
		sessionManager,
		securityLogger,
		config,
		tenantToolSet,
	)
	mh.SetHelpAnalyzer(helpAnalyzer)

	// Verify help analyzer is set correctly
	assert.NotNil(t, mh.helpAnalyzer)
	assert.Equal(t, helpAnalyzer, mh.helpAnalyzer)

	// Test intent recognition failure scenario
	message := "invalid command that doesn't match any patterns"
	intent, err := intentProcessor.ProcessMessage(message)

	// Should return nil intent (not matched) and no error
	assert.NoError(t, err)
	assert.Nil(t, intent)
}

func TestMessageHandler_BasicHelpFallback(t *testing.T) {
	// Create message handler without help analyzer to test fallback
	mh := NewMessageHandler(
		&IntentProcessor{},
		nil, // No Slack connection
		&mockQueryHandler{},
		&ToolManagerImpl{},
		&SessionManager{},
		sec.NewSecurityLogger(),
		&config.File{},
		&query.TenantToolSet{},
	)
	// Note: No help analyzer set

	// Verify help analyzer is nil
	assert.Nil(t, mh.helpAnalyzer)

	// Test intent recognition failure scenario
	message := "some unrecognized command"
	intent, err := (&IntentProcessor{}).ProcessMessage(message)

	// Should return nil intent (not matched) and no error
	assert.NoError(t, err)
	assert.Nil(t, intent)
}

// mockQueryHandler is a simple mock for testing
type mockQueryHandler struct{}

func (m *mockQueryHandler) HandleQueryWithUpdater(ctx context.Context, slackCtx *SlackContext, session *UserSession, message string, updater *SlackUpdater) error {
	// Simply return nil for testing purposes
	return nil
}

func TestMessageHandler_ValidIntentNoHelp(t *testing.T) {
	// Test that valid intents are recognized properly
	intentProcessor := NewIntentProcessor()

	// Test a valid intent that should be recognized
	message := "show preferences"
	intent, err := intentProcessor.ProcessMessage(message)

	// Should return a valid intent with high confidence
	assert.NoError(t, err)
	assert.NotNil(t, intent)
	assert.Equal(t, "show_preferences", intent.Action)
	assert.GreaterOrEqual(t, intent.Confidence, 0.7)
}
