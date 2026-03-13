package config

// CronStanza describes a message to be dispatched on a schedule to a specific user.
type CronStanza struct {
	// Title is the name of the cron stanza
	Title string `hcl:"name,label"`
	// Schedule is the cron schedule expression
	Schedule string `hcl:"schedule"`
	// SendTo is to dispatch the cron job to
	SendTo string `hcl:"send_to"`
	// Message is the message to dispatch for on the schedule
	Message string `hcl:"message"`
}
