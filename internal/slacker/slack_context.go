package slacker

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
