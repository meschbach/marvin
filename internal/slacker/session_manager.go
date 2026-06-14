package slacker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/junk"
	"github.com/meschbach/marvin/internal/llm"
	"github.com/meschbach/marvin/internal/query"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// SessionManager handles user sessions with persistence
type SessionManager struct {
	sessions  sync.Map // "userID:channelID" -> *UserSession
	storePath string
}

// NewSessionManager creates a new session manager
func NewSessionManager(storePath string) (*SessionManager, error) {
	// Ensure store directory exists
	if err := os.MkdirAll(storePath, 0755); err != nil {
		return nil, fmt.Errorf("creating session store directory: %w", err)
	}

	sm := &SessionManager{
		storePath: storePath,
		sessions:  sync.Map{},
	}

	// Load existing sessions
	if err := sm.loadAllSessions(); err != nil {
		return nil, fmt.Errorf("loading existing sessions: %w", err)
	}

	return sm, nil
}

// GetOrCreateSession gets an existing session or creates a new one
func (sm *SessionManager) GetOrCreateSession(ctx context.Context, userID, channelID string, userContext *query.UserContext) *UserSession {
	ctx, span := tracer.Start(ctx, "slacker.SessionManager.GetOrCreateSession",
		trace.WithAttributes(attribute.String("user.id", userID), attribute.String("channel.id", channelID)),
	)
	defer span.End()
	sessionKey := fmt.Sprintf("%s:%s", userID, channelID)

	// Try to get existing session
	if session, exists := sm.sessions.Load(sessionKey); exists {
		if userSession, convertible := session.(*UserSession); convertible {
			span.SetAttributes(attribute.Bool("found", true))
			userSession.LastActivity = time.Now()
			userSession.UserContext = userContext
			return userSession
		}
	}
	span.SetAttributes(attribute.Bool("found", false))

	// Try to load existing preferences for this user
	preferences := DefaultUserPreferences()
	if userPrefs, hasPrefs := sm.GetPreferences(userID); hasPrefs {
		preferences = userPrefs
	}

	// Create new session with loaded preferences
	newSession := NewUserSessionWithPreferences(userID, channelID, userContext, preferences)

	// Store session
	sm.sessions.Store(sessionKey, newSession)

	// Persist to disk
	if err := sm.saveSession(newSession); err != nil {
		junk.RecordSpanErrorNoLint(span, err)
		// Log error but continue
		fmt.Fprintf(os.Stderr, "Warning: failed to save new session: %v\n", err)
	}

	return newSession
}

// GetSession retrieves an existing session
func (sm *SessionManager) GetSession(userID, channelID string) (*UserSession, bool) {
	sessionKey := fmt.Sprintf("%s:%s", userID, channelID)
	if session, exists := sm.sessions.Load(sessionKey); exists {
		userSession, convertible := session.(*UserSession)
		return userSession, convertible
	}
	return nil, false
}

// AddMessage adds a message to a session
func (sm *SessionManager) AddMessage(userID, channelID string, message llm.Message) error {
	userSession, exists := sm.GetSession(userID, channelID)
	if !exists {
		return fmt.Errorf("session not found for user %s in channel %s", userID, channelID)
	}

	userSession.Messages = append(userSession.Messages, message)
	userSession.LastActivity = time.Now()

	return sm.saveSession(userSession)
}

// ClearSession clears all messages in a session
func (sm *SessionManager) ClearSession(userID, channelID string) error {
	userSession, exists := sm.GetSession(userID, channelID)
	if !exists {
		return fmt.Errorf("session not found for user %s in channel %s", userID, channelID)
	}

	userSession.Messages = []llm.Message{}
	userSession.LastActivity = time.Now()

	return sm.saveSession(userSession)
}

// SetThreadTS sets the thread timestamp for a session
func (sm *SessionManager) SetThreadTS(userID, channelID, threadTS string) error {
	userSession, exists := sm.GetSession(userID, channelID)
	if !exists {
		return fmt.Errorf("session not found for user %s in channel %s", userID, channelID)
	}

	userSession.ThreadTS = threadTS
	return sm.saveSession(userSession)
}

// UpdateAvailableTools updates the list of available tools for a session
func (sm *SessionManager) UpdateAvailableTools(userID, channelID string, tools []string) error {
	userSession, exists := sm.GetSession(userID, channelID)
	if !exists {
		return fmt.Errorf("session not found for user %s in channel %s", userID, channelID)
	}

	userSession.AvailableTools = tools
	userSession.LastActivity = time.Now()

	return sm.saveSession(userSession)
}

// GetPreferences returns a user's preferences from any of their sessions
func (sm *SessionManager) GetPreferences(userID string) (UserPreferences, bool) {
	var foundPrefs UserPreferences
	found := false

	// Look through all sessions for this user to find their preferences
	sm.sessions.Range(func(key, value interface{}) bool {
		if userSession, convertible := value.(*UserSession); convertible {
			if userSession.UserID == userID {
				foundPrefs = userSession.Preferences
				found = true
				return false // Found what we need, stop iteration
			}
		}
		return true
	})

	return foundPrefs, found
}

// UpdatePreferences updates a user's preferences across all their sessions
func (sm *SessionManager) UpdatePreferences(userID string, preferences UserPreferences) error {
	var updateErrors []error
	foundSession := false

	// Update preferences in all sessions for this user
	sm.sessions.Range(func(key, value interface{}) bool {
		if userSession, convertible := value.(*UserSession); convertible {
			if userSession.UserID == userID {
				foundSession = true
				userSession.SetPreferences(preferences)
				if err := sm.saveSession(userSession); err != nil {
					updateErrors = append(updateErrors, err)
				}
			}
		}
		return true
	})

	// If no session exists for this user, create a temporary one to store preferences
	if !foundSession {
		tempSession := &UserSession{
			UserID:      userID,
			ChannelID:   "temp-channel",
			Preferences: preferences,
		}
		// Add to in-memory sessions
		sessionKey := fmt.Sprintf("%s:%s", userID, "temp-channel")
		sm.sessions.Store(sessionKey, tempSession)

		if err := sm.saveSession(tempSession); err != nil {
			return fmt.Errorf("failed to save user preferences: %v", err)
		}
	}

	if len(updateErrors) > 0 {
		return fmt.Errorf("failed to update some sessions: %v", updateErrors)
	}

	return nil
}

// ResolveUserPreferences resolves preferences in hierarchy: user session > HCL config > defaults
func (sm *SessionManager) ResolveUserPreferences(userID string, config *config.File) UserPreferences {
	// Start with default preferences
	resolvedPrefs := DefaultUserPreferences()

	// 1. Apply HCL configuration defaults
	if config != nil {
		resolvedPrefs.ShowThinking = config.ShowThinking()
		resolvedPrefs.ShowTools = config.ShowTools()
		resolvedPrefs.ShowDone = config.ShowDone()
		resolvedPrefs.ThinkingFormat = config.ThinkingFormat()
		resolvedPrefs.ToolFormat = config.ToolFormat()
		resolvedPrefs.Verbose = config.Verbose()
	}

	// 2. Apply user preferences (they take priority over HCL config)
	userPrefs, hasUserPrefs := sm.GetPreferences(userID)
	if hasUserPrefs {
		resolvedPrefs.ShowThinking = userPrefs.ShowThinking
		resolvedPrefs.ShowTools = userPrefs.ShowTools
		resolvedPrefs.ShowDone = userPrefs.ShowDone
		resolvedPrefs.ThinkingFormat = userPrefs.ThinkingFormat
		resolvedPrefs.ToolFormat = userPrefs.ToolFormat
		resolvedPrefs.Verbose = userPrefs.Verbose
	}

	return resolvedPrefs
}

// ListSessions returns all active sessions
func (sm *SessionManager) ListSessions() []*UserSession {
	var sessions []*UserSession
	sm.sessions.Range(func(key, value interface{}) bool {
		if userSession, ok := value.(*UserSession); ok {
			sessions = append(sessions, userSession)
		}
		return true
	})
	return sessions
}

// CleanupOldSessions removes sessions inactive for more than the specified duration
func (sm *SessionManager) CleanupOldSessions(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)

	sm.sessions.Range(func(key, value interface{}) bool {
		if userSession, convertible := value.(*UserSession); convertible {
			if userSession.LastActivity.Before(cutoff) {
				sm.sessions.Delete(key)

				// Remove session file
				filename := filepath.Join(sm.storePath, fmt.Sprintf("session-%s-%s.json", userSession.UserID, userSession.ChannelID))
				if removeErr := os.Remove(filename); removeErr != nil {
					// Log cleanup error but don't stop cleanup process
					fmt.Fprintf(os.Stderr, "Warning: failed to remove session file %s: %v\n", filename, removeErr)
				} // Ignore errors
			}
		}
		return true
	})
}

// saveSession persists a session to disk
func (sm *SessionManager) saveSession(session *UserSession) error {
	filename := filepath.Join(sm.storePath, fmt.Sprintf("session-%s-%s.json", session.UserID, session.ChannelID))

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	// Write to temporary file first
	tempFile := filename + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("writing temp session file: %w", err)
	}

	// Rename to final file
	if err := os.Rename(tempFile, filename); err != nil {
		if removeErr := os.Remove(tempFile); removeErr != nil {
			// Log cleanup error but don't overwrite the main error
			fmt.Fprintf(os.Stderr, "Warning: failed to remove temp file %s: %v\n", tempFile, removeErr)
		} // Clean up temp file
		return fmt.Errorf("renaming session file: %w", err)
	}

	return nil
}

// loadAllSessions loads all existing sessions from disk
//
//nolint:gocyclo
func (sm *SessionManager) loadAllSessions() error {
	files, err := os.ReadDir(sm.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No sessions directory yet
		}
		return fmt.Errorf("reading session directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || len(file.Name()) < 8 || file.Name()[0:8] != "session-" || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		filename := filepath.Join(sm.storePath, file.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to read session file %s: %v\n", filename, err)
			continue
		}

		var session UserSession
		if err := json.Unmarshal(data, &session); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to unmarshal session file %s: %v\n", filename, err)
			continue
		}

		// Store session in memory
		sessionKey := fmt.Sprintf("%s:%s", session.UserID, session.ChannelID)
		sm.sessions.Store(sessionKey, &session)
	}

	return nil
}
