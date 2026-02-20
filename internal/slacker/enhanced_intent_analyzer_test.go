package slacker

import (
	"strings"
	"testing"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	"github.com/stretchr/testify/assert"
)

func TestEnhancedIntentAnalyzer_BasicFunctionality(t *testing.T) {
	// Create a base help analyzer
	helpAnalyzer := NewHelpAnalyzer(
		nil, // LLM interface (nil for testing)
		&config.File{},
		&SessionManager{},
		&ToolManagerImpl{},
		&query.TenantToolSet{},
	)

	// Create enhanced analyzer
	enhanced := NewEnhancedIntentAnalyzer(helpAnalyzer)

	// Should not be nil
	assert.NotNil(t, enhanced)
	assert.NotNil(t, enhanced.helpAnalyzer)
}

func TestEnhancedIntentAnalyzer_SimilarCommands(t *testing.T) {
	helpAnalyzer := NewHelpAnalyzer(nil, &config.File{}, &SessionManager{}, &ToolManagerImpl{}, &query.TenantToolSet{})
	enhanced := NewEnhancedIntentAnalyzer(helpAnalyzer)

	// Create help context
	helpCtx := &HelpContext{
		UserID:          "U123",
		ChannelID:       "C456",
		OriginalMessage: "lsit",
		AvailableTools:  []string{"http:api-client", "local:git"},
		AvailableModels: []string{"ministral-3:3b"},
		IsAdmin:         false,
		RecentCommands:  []string{"list my tools", "add http tool at https://api.example.com/mcp"},
	}

	// Test similar command detection
	similar := enhanced.findSimilarCommands("lsit my tools", helpCtx)

	// Should find similar commands
	assert.GreaterOrEqual(t, len(similar), 1)

	// Check that "list my tools" is found as similar
	found := false
	for _, cmd := range similar {
		if strings.Contains(cmd.Command, "list") {
			found = true
			assert.Greater(t, cmd.Similarity, 0.3)
			break
		}
	}
	assert.True(t, found, "Should find 'list' command as similar to 'lsit'")
}

func TestEnhancedIntentAnalyzer_TypoCorrections(t *testing.T) {
	helpAnalyzer := NewHelpAnalyzer(nil, &config.File{}, &SessionManager{}, &ToolManagerImpl{}, &query.TenantToolSet{})
	enhanced := NewEnhancedIntentAnalyzer(helpAnalyzer)

	helpCtx := &HelpContext{
		UserID:          "U123",
		ChannelID:       "C456",
		OriginalMessage: "shwo preferences",
		AvailableTools:  []string{"http:api-client"},
		AvailableModels: []string{"ministral-3:3b"},
		IsAdmin:         false,
		RecentCommands:  []string{},
	}

	// Test typo correction
	corrections := enhanced.findTypoCorrections("shwo preferences", helpCtx)

	// Should find "show preferences" as a correction
	assert.Contains(t, corrections, "show preferences")
}

func TestEnhancedIntentAnalyzer_PatternCompletions(t *testing.T) {
	helpAnalyzer := NewHelpAnalyzer(nil, &config.File{}, &SessionManager{}, &ToolManagerImpl{}, &query.TenantToolSet{})
	enhanced := NewEnhancedIntentAnalyzer(helpAnalyzer)

	helpCtx := &HelpContext{
		UserID:          "U123",
		ChannelID:       "C456",
		OriginalMessage: "add http",
		AvailableTools:  []string{"http:api-client"},
		AvailableModels: []string{"ministral-3:3b"},
		IsAdmin:         false,
		RecentCommands:  []string{},
	}

	// Test pattern completion
	completions := enhanced.findPatternCompletions("add http", helpCtx)

	// Should suggest the complete command
	assert.Contains(t, completions, "add http tool at <url>")
}

func TestEnhancedIntentAnalyzer_ContextualSuggestions(t *testing.T) {
	helpAnalyzer := NewHelpAnalyzer(nil, &config.File{}, &SessionManager{}, &ToolManagerImpl{}, &query.TenantToolSet{})
	enhanced := NewEnhancedIntentAnalyzer(helpAnalyzer)

	// Test with user who has recent tool commands
	helpCtx := &HelpContext{
		UserID:          "U123",
		ChannelID:       "C456",
		OriginalMessage: "help",
		AvailableTools:  []string{"http:api-client"},
		AvailableModels: []string{"ministral-3:3b"},
		IsAdmin:         false,
		RecentCommands:  []string{"add http tool at https://api.example.com/mcp", "list my tools"},
	}

	suggestions := enhanced.findContextualSuggestions("help", helpCtx)

	// Should suggest tool-related commands
	assert.GreaterOrEqual(t, len(suggestions), 1)

	// Check for tool management suggestions
	foundToolSuggestion := false
	for _, suggestion := range suggestions {
		if strings.Contains(suggestion, "list my tools") || strings.Contains(suggestion, "add http tool") {
			foundToolSuggestion = true
			break
		}
	}
	assert.True(t, foundToolSuggestion, "Should suggest tool-related commands for users with recent tool activity")

	// Test with admin user
	helpCtx.IsAdmin = true
	adminSuggestions := enhanced.findContextualSuggestions("help", helpCtx)

	// Should suggest admin commands
	foundAdminSuggestion := false
	for _, suggestion := range adminSuggestions {
		if strings.Contains(suggestion, "model access") {
			foundAdminSuggestion = true
			break
		}
	}
	assert.True(t, foundAdminSuggestion, "Should suggest admin commands for admin users")
}

func TestEnhancedIntentAnalyzer_EditDistance(t *testing.T) {
	helpAnalyzer := NewHelpAnalyzer(nil, &config.File{}, &SessionManager{}, &ToolManagerImpl{}, &query.TenantToolSet{})
	enhanced := NewEnhancedIntentAnalyzer(helpAnalyzer)

	// Test edit distance calculation
	assert.Equal(t, 0, enhanced.editDistance("", ""))
	assert.Equal(t, 1, enhanced.editDistance("a", ""))
	assert.Equal(t, 1, enhanced.editDistance("", "a"))
	assert.Equal(t, 0, enhanced.editDistance("test", "test"))
	assert.Equal(t, 1, enhanced.editDistance("test", "tests"))
	assert.Equal(t, 1, enhanced.editDistance("test", "tost"))
	assert.Equal(t, 2, enhanced.editDistance("test", "toast"))
}

func TestEnhancedIntentAnalyzer_FullEnhancedAnalysis(t *testing.T) {
	helpAnalyzer := NewHelpAnalyzer(nil, &config.File{}, &SessionManager{}, &ToolManagerImpl{}, &query.TenantToolSet{})
	enhanced := NewEnhancedIntentAnalyzer(helpAnalyzer)

	helpCtx := &HelpContext{
		UserID:          "U123",
		ChannelID:       "C456",
		OriginalMessage: "lsit my tool",
		AvailableTools:  []string{"http:api-client", "local:git"},
		AvailableModels: []string{"ministral-3:3b"},
		IsAdmin:         false,
		RecentCommands:  []string{"list my tools"},
	}

	// Test full enhanced analysis
	analysis, err := enhanced.AnalyzeIntentFailureEnhanced(t.Context(), "lsit my tool", helpCtx)

	// Should not error and should provide enhanced analysis
	assert.NoError(t, err)
	assert.NotNil(t, analysis)
	assert.Equal(t, "intent_failure", analysis.FailureType)
	assert.NotEmpty(t, analysis.Diagnosis)

	// Should have enhanced suggestions (more than base analysis)
	assert.GreaterOrEqual(t, len(analysis.Suggestions), 1)

	// Check if any suggestions mention typo corrections or similar commands
	foundEnhancedSuggestion := false
	for _, suggestion := range analysis.Suggestions {
		if strings.Contains(strings.ToLower(suggestion), "did you mean") ||
			strings.Contains(strings.ToLower(suggestion), "corrected typo") {
			foundEnhancedSuggestion = true
			break
		}
	}
	assert.True(t, foundEnhancedSuggestion, "Should provide enhanced suggestions with corrections")
}
