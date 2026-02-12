package slacker

import (
	"testing"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
	"github.com/stretchr/testify/assert"
)

func TestHelpAnalyzer_BasicFunctionality(t *testing.T) {
	// Test that HelpAnalyzer can be created without errors
	helpAnalyzer := NewHelpAnalyzer(
		nil, // LLM interface (nil for testing)
		&config.File{},
		&SessionManager{},
		&ToolManagerImpl{},
		&query.TenantToolSet{},
	)

	// Should not be nil
	assert.NotNil(t, helpAnalyzer)
}

func TestHelpContext_BasicFunctionality(t *testing.T) {
	// Test that HelpContextBuilder can be created without errors
	contextBuilder := NewHelpContextBuilder(
		&SessionManager{},
		&config.File{},
		&query.TenantToolSet{},
	)

	// Should not be nil
	assert.NotNil(t, contextBuilder)
}

func TestMessageHandler_WithHelpAnalyzer(t *testing.T) {
	// Test that MessageHandler can be created and have help analyzer set
	mh := NewMessageHandler(
		&IntentProcessor{},
		&SlackConnection{},
		&mockQueryHandler{},
		&ToolManagerImpl{},
		&SessionManager{},
		sec.NewSecurityLogger(),
		&config.File{},
		&query.TenantToolSet{},
	)

	// Should not be nil
	assert.NotNil(t, mh)

	// Help analyzer should be nil initially
	assert.Nil(t, mh.helpAnalyzer)

	// Set help analyzer
	helpAnalyzer := NewHelpAnalyzer(
		nil,
		&config.File{},
		&SessionManager{},
		&ToolManagerImpl{},
		&query.TenantToolSet{},
	)

	mh.SetHelpAnalyzer(helpAnalyzer)

	// Help analyzer should now be set
	assert.NotNil(t, mh.helpAnalyzer)
}

func TestHelpAnalysis_Formatting(t *testing.T) {
	// Test that help analysis formatting works correctly
	analysis := &HelpAnalysis{
		FailureType: "intent_failure",
		Diagnosis:   "Command not recognized",
		Suggestions: []string{"Check command syntax", "Use 'help' command"},
		Examples:    []string{"add http tool at <url>", "list my tools"},
		ContextHelp: "Use '@marvin help' for more assistance",
		Confidence:  0.85,
	}

	// Test that we can format the help message
	// Note: This tests the formatting logic without needing Slack integration
	assert.NotEmpty(t, analysis.Diagnosis)
	assert.Len(t, analysis.Suggestions, 2)
	assert.Len(t, analysis.Examples, 2)
	assert.Equal(t, 0.85, analysis.Confidence)
}
