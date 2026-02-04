package slacker

import ()

// SlackContext provides context for Slack operations
type SlackContext struct {
	UserID    string
	UserName  string
	ChannelID string
	TeamID    string
	Message   string
	Timestamp string
	ThreadTS  string
}

// getNameFromConfig extracts name from tool configuration
func getNameFromConfig(config interface{}) string {
	// Use type assertion safely
	if cfg, ok := config.(interface{ GetName() string }); ok {
		return cfg.GetName()
	}
	return "unknown"
}
