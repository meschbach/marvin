package cron

import "context"

type ScheduledEvent interface {
	Cancel(ctx context.Context) error
}

type Job interface {
	OnCron()
}

// Scheduler provides a seam for scheduling crontab specs at runtime for invoking notify.  A ScheduledEvent is returned
// which can terminate the dispatching of an event.
type Scheduler interface {
	Schedule(ctx context.Context, tabSpec string, notify Job) (ScheduledEvent, error)
}
