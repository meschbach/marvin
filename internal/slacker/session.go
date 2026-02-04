package slacker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/meschbach/marvin/internal/query"
	"github.com/ollama/ollama/api"
)

// UserSession represents a user's conversation session
type UserSession struct {
	UserID         string             `json:"user_id"`
	ChannelID      string             `json:"channel_id"`
	ThreadTS       string             `json:"thread_ts,omitempty"`
	LastActivity   time.Time          `json:"last_activity"`
	Messages       []api.Message      `json:"messages"`
	AvailableTools []string           `json:"available_tools"`
	ToolNamespace  string             `json:"tool_namespace"`
	UserContext    *query.UserContext `json:"-"` // Not serialized
	mutex          sync.RWMutex
}

// GetAvailableTools returns the list of available tools for this session
func (us *UserSession) GetAvailableTools() []string {
	us.mutex.RLock()
	defer us.mutex.RUnlock()

	// Return a copy to prevent modification
	tools := make([]string, len(us.AvailableTools))
	copy(tools, us.AvailableTools)
	return tools
}

// AddMessage adds a message to the session
func (us *UserSession) AddMessage(message api.Message) {
	us.mutex.Lock()
	defer us.mutex.Unlock()

	us.Messages = append(us.Messages, message)
	us.LastActivity = time.Now()
}

// SetThreadTS sets the thread timestamp for the session
func (us *UserSession) SetThreadTS(threadTS string) {
	us.mutex.Lock()
	defer us.mutex.Unlock()

	us.ThreadTS = threadTS
}

// SessionManager handles user sessions with persistence
type SessionManager struct {
	sessions  sync.Map // "userID:channelID" -> *UserSession
	storePath string
	mutex     sync.RWMutex
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
		mutex:     sync.RWMutex{},
	}

	// Load existing sessions
	if err := sm.loadAllSessions(); err != nil {
		return nil, fmt.Errorf("loading existing sessions: %w", err)
	}

	return sm, nil
}

// GetOrCreateSession gets an existing session or creates a new one
func (sm *SessionManager) GetOrCreateSession(userID, channelID string, userContext *query.UserContext) *UserSession {
	sessionKey := fmt.Sprintf("%s:%s", userID, channelID)

	// Try to get existing session
	if session, exists := sm.sessions.Load(sessionKey); exists {
		userSession := session.(*UserSession)
		userSession.mutex.Lock()
		userSession.LastActivity = time.Now()
		userSession.UserContext = userContext
		userSession.mutex.Unlock()
		return userSession
	}

	// Create new session
	newSession := &UserSession{
		UserID:         userID,
		ChannelID:      channelID,
		LastActivity:   time.Now(),
		Messages:       []api.Message{},
		AvailableTools: []string{},
		ToolNamespace:  fmt.Sprintf("user-%s", userID),
		UserContext:    userContext,
	}

	// Store session
	sm.sessions.Store(sessionKey, newSession)

	// Persist to disk
	if err := sm.saveSession(newSession); err != nil {
		// Log error but continue
		fmt.Fprintf(os.Stderr, "Warning: failed to save new session: %v\n", err)
	}

	return newSession
}

// GetSession retrieves an existing session
func (sm *SessionManager) GetSession(userID, channelID string) (*UserSession, bool) {
	sessionKey := fmt.Sprintf("%s:%s", userID, channelID)
	if session, exists := sm.sessions.Load(sessionKey); exists {
		return session.(*UserSession), true
	}
	return nil, false
}

// AddMessage adds a message to a session
func (sm *SessionManager) AddMessage(userID, channelID string, message api.Message) error {
	userSession, exists := sm.GetSession(userID, channelID)
	if !exists {
		return fmt.Errorf("session not found for user %s in channel %s", userID, channelID)
	}

	userSession.mutex.Lock()
	defer userSession.mutex.Unlock()

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

	userSession.mutex.Lock()
	defer userSession.mutex.Unlock()

	userSession.Messages = []api.Message{}
	userSession.LastActivity = time.Now()

	return sm.saveSession(userSession)
}

// SetThreadTS sets the thread timestamp for a session
func (sm *SessionManager) SetThreadTS(userID, channelID, threadTS string) error {
	userSession, exists := sm.GetSession(userID, channelID)
	if !exists {
		return fmt.Errorf("session not found for user %s in channel %s", userID, channelID)
	}

	userSession.mutex.Lock()
	defer userSession.mutex.Unlock()

	userSession.ThreadTS = threadTS
	return sm.saveSession(userSession)
}

// UpdateAvailableTools updates the list of available tools for a session
func (sm *SessionManager) UpdateAvailableTools(userID, channelID string, tools []string) error {
	userSession, exists := sm.GetSession(userID, channelID)
	if !exists {
		return fmt.Errorf("session not found for user %s in channel %s", userID, channelID)
	}

	userSession.mutex.Lock()
	defer userSession.mutex.Unlock()

	userSession.AvailableTools = tools
	userSession.LastActivity = time.Now()

	return sm.saveSession(userSession)
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
		userSession := value.(*UserSession)
		if userSession.LastActivity.Before(cutoff) {
			sm.sessions.Delete(key)

			// Remove session file
			filename := filepath.Join(sm.storePath, fmt.Sprintf("session-%s-%s.json", userSession.UserID, userSession.ChannelID))
			os.Remove(filename) // Ignore errors
		}
		return true
	})
}

// saveSession persists a session to disk
func (sm *SessionManager) saveSession(session *UserSession) error {
	session.mutex.RLock()
	defer session.mutex.RUnlock()

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
		os.Remove(tempFile) // Clean up temp file
		return fmt.Errorf("renaming session file: %w", err)
	}

	return nil
}

// loadAllSessions loads all existing sessions from disk
func (sm *SessionManager) loadAllSessions() error {
	files, err := os.ReadDir(sm.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No sessions directory yet
		}
		return fmt.Errorf("reading session directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || len(file.Name()) < 7 || file.Name()[0:7] != "session-" || filepath.Ext(file.Name()) != ".json" {
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
