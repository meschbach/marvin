package slacker

import (
	"github.com/meschbach/marvin/internal/slacker/security"
)

// SlackLoggerAdapter adapts the security.SecurityLogger to implement the query.Logger interface
type SlackLoggerAdapter struct {
	securityLogger *security.SecurityLogger
	userID         string
}

// NewSlackLoggerAdapter creates a new adapter that wraps security.SecurityLogger
func NewSlackLoggerAdapter(securityLogger *security.SecurityLogger, userID string) *SlackLoggerAdapter {
	return &SlackLoggerAdapter{
		securityLogger: securityLogger,
		userID:         userID,
	}
}

// Debug implements the query.Logger interface
func (s *SlackLoggerAdapter) Debug(_, component, message string) {
	if s.securityLogger != nil {
		s.securityLogger.LogDebug(s.userID, component, message)
	}
}

// Error implements the query.Logger interface
func (s *SlackLoggerAdapter) Error(_, component, message string) {
	if s.securityLogger != nil {
		s.securityLogger.LogError(s.userID, component, message)
	}
}
