package slacker

import (
	"os"
	"testing"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPreferenceResolution_Integration tests the complete preference resolution workflow
func TestPreferenceResolution_Integration(t *testing.T) {
	// Create temporary directory for session storage
	tempDir, err := os.MkdirTemp("", "marvin-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create session manager
	sessionManager, err := NewSessionManager(tempDir)
	require.NoError(t, err)

	// Create test config with display settings
	testConfig := &config.File{
		Display: &config.DisplayBlock{
			ShowThinking:   &[]bool{true}[0],
			ShowTools:      &[]bool{false}[0],
			ShowDone:       &[]bool{true}[0],
			ThinkingFormat: &[]string{"markdown"}[0],
			ToolFormat:     &[]string{"simple"}[0],
			Verbose:        &[]bool{true}[0],
		},
	}

	t.Run("NewUserWithoutPreferences", func(t *testing.T) {
		userID := "test-user-1"

		// Resolve preferences for new user (should use config defaults)
		resolved := sessionManager.ResolveUserPreferences(userID, testConfig)

		// Should use HCL config values since user has no preferences
		assert.True(t, resolved.ShowThinking, "Should use config ShowThinking=true")
		assert.False(t, resolved.ShowTools, "Should use config ShowTools=false")
		assert.True(t, resolved.ShowDone, "Should use config ShowDone=true")
		assert.Equal(t, "markdown", resolved.ThinkingFormat, "Should use config ThinkingFormat=markdown")
		assert.Equal(t, "simple", resolved.ToolFormat, "Should use config ToolFormat=simple")
		assert.True(t, resolved.Verbose, "Should use config Verbose=true")
	})

	t.Run("UserWithExistingPreferences", func(t *testing.T) {
		userID := "test-user-2"

		// Set user preferences that differ from config
		userPrefs := UserPreferences{
			ShowThinking:   false,      // Different from config (true)
			ShowTools:      true,       // Different from config (false)
			ShowDone:       false,      // Different from config (true)
			ThinkingFormat: "plain",    // Different from config (markdown)
			ToolFormat:     "detailed", // Different from config (simple)
			Verbose:        false,      // Different from config (true)
		}

		err := sessionManager.UpdatePreferences(userID, userPrefs)
		require.NoError(t, err)

		// Resolve preferences (should use user preferences over config)
		resolved := sessionManager.ResolveUserPreferences(userID, testConfig)

		// Should use user preferences since they exist
		assert.False(t, resolved.ShowThinking, "Should use user preference ShowThinking=false")
		assert.True(t, resolved.ShowTools, "Should use user preference ShowTools=true")
		assert.False(t, resolved.ShowDone, "Should use user preference ShowDone=false")
		assert.Equal(t, "plain", resolved.ThinkingFormat, "Should use user preference ThinkingFormat=plain")
		assert.Equal(t, "detailed", resolved.ToolFormat, "Should use user preference ToolFormat=detailed")
		assert.False(t, resolved.Verbose, "Should use user preference Verbose=false")
	})

	t.Run("UserWithNoConfig", func(t *testing.T) {
		userID := "test-user-3"

		// Resolve preferences with no config (should use user preferences or defaults)
		resolved := sessionManager.ResolveUserPreferences(userID, nil)

		// Should use hard-coded defaults since no config and no user preferences
		defaults := DefaultUserPreferences()
		assert.Equal(t, defaults.ShowThinking, resolved.ShowThinking)
		assert.Equal(t, defaults.ShowTools, resolved.ShowTools)
		assert.Equal(t, defaults.ShowDone, resolved.ShowDone)
		assert.Equal(t, defaults.ThinkingFormat, resolved.ThinkingFormat)
		assert.Equal(t, defaults.ToolFormat, resolved.ToolFormat)
		assert.Equal(t, defaults.Verbose, resolved.Verbose)
	})

	t.Run("PreferencePersistence", func(t *testing.T) {
		userID := "test-user-4"

		// Create a session
		userContext := &query.UserContext{
			UserID: userID,
		}
		session := sessionManager.GetOrCreateSession(userID, "test-channel", userContext)

		// Update preferences
		newPrefs := UserPreferences{
			ShowThinking:   true,
			ShowTools:      false,
			ShowDone:       true,
			ThinkingFormat: "collapsed",
			ToolFormat:     "simple",
			Verbose:        true,
		}

		session.SetPreferences(newPrefs)

		// Verify preferences are persisted
		retrievedPrefs, exists := sessionManager.GetPreferences(userID)
		require.True(t, exists, "Preferences should exist")
		assert.Equal(t, newPrefs, retrievedPrefs, "Retrieved preferences should match set preferences")

		// Test persistence across sessions
		session2 := sessionManager.GetOrCreateSession(userID, "test-channel-2", userContext)
		assert.Equal(t, newPrefs, session2.GetPreferences(), "Preferences should persist across sessions")
	})
}

// TestPreferenceIntentHandling_Integration tests preference intent handling
func TestPreferenceIntentHandling_Integration(t *testing.T) {
	// Create temporary directory for session storage
	tempDir, err := os.MkdirTemp("", "marvin-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create session manager
	sessionManager, err := NewSessionManager(tempDir)
	require.NoError(t, err)

	userID := "test-user-intent"

	t.Run("ToggleThinkingIntent", func(t *testing.T) {
		// Test toggling thinking on
		intent := &ToolManagementIntent{
			Action: "toggle_thinking",
			Config: "on",
		}

		response, err := HandlePreferenceIntent(intent, sessionManager, userID)
		require.NoError(t, err)
		assert.Contains(t, response, "enabled")

		// Verify preference was saved
		prefs, exists := sessionManager.GetPreferences(userID)
		require.True(t, exists)
		assert.True(t, prefs.ShowThinking)

		// Test toggling thinking off
		intent.Config = "off"
		response, err = HandlePreferenceIntent(intent, sessionManager, userID)
		require.NoError(t, err)
		assert.Contains(t, response, "disabled")

		// Verify preference was updated
		prefs, _ = sessionManager.GetPreferences(userID)
		assert.False(t, prefs.ShowThinking)
	})

	t.Run("SetThinkingFormatIntent", func(t *testing.T) {
		intent := &ToolManagementIntent{
			Action: "set_thinking_format",
			Config: "markdown",
		}

		response, err := HandlePreferenceIntent(intent, sessionManager, userID)
		require.NoError(t, err)
		assert.Contains(t, response, "markdown")

		// Verify preference was saved
		prefs, _ := sessionManager.GetPreferences(userID)
		assert.Equal(t, "markdown", prefs.ThinkingFormat)
	})

	t.Run("ShowPreferencesIntent", func(t *testing.T) {
		// Set some preferences first
		testPrefs := UserPreferences{
			ShowThinking:   true,
			ShowTools:      false,
			ShowDone:       true,
			ThinkingFormat: "collapsed",
			ToolFormat:     "simple",
			Verbose:        true,
		}
		err := sessionManager.UpdatePreferences(userID, testPrefs)
		require.NoError(t, err)

		intent := &ToolManagementIntent{
			Action: "show_preferences",
		}

		response, err := HandlePreferenceIntent(intent, sessionManager, userID)
		require.NoError(t, err)
		assert.Contains(t, response, "Current preferences")
		assert.Contains(t, response, "true")
		assert.Contains(t, response, "false")
		assert.Contains(t, response, "collapsed")
		assert.Contains(t, response, "simple")
	})
}

// TestSessionManager_Persistence tests that preferences persist across restarts
func TestSessionManager_Persistence(t *testing.T) {
	// Create temporary directory for session storage
	tempDir, err := os.MkdirTemp("", "marvin-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	userID := "test-user-persistence"

	// Create session manager and set preferences
	sessionManager1, err := NewSessionManager(tempDir)
	require.NoError(t, err)

	testPrefs := UserPreferences{
		ShowThinking:   true,
		ShowTools:      false,
		ShowDone:       true,
		ThinkingFormat: "markdown",
		ToolFormat:     "simple",
		Verbose:        true,
	}

	err = sessionManager1.UpdatePreferences(userID, testPrefs)
	require.NoError(t, err)

	// Create new session manager (simulating restart)
	sessionManager2, err := NewSessionManager(tempDir)
	require.NoError(t, err)

	// Verify preferences persisted
	retrievedPrefs, exists := sessionManager2.GetPreferences(userID)
	require.True(t, exists)
	assert.Equal(t, testPrefs, retrievedPrefs)
}
