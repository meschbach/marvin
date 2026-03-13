// Package robfig provides an implementation of the Cron interface using the github.com/robfig/cron library.
package robfig

import (
	"context"

	scron "github.com/meschbach/marvin/internal/slacker/cron"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	core *cron.Cron
}

func (s *Scheduler) Schedule(ctx context.Context, tabSpec string, notify scron.Job) (scron.ScheduledEvent, error) {
	entity, err := s.core.AddFunc(tabSpec, func() {
		notify.OnCron()
	})
	if err != nil {
		return nil, err
	}
	return &Job{
		scheduler: s,
		id:        entity,
	}, nil
}

var _ scron.Scheduler = (*Scheduler)(nil)

func NewScheduler() *Scheduler {
	c := cron.New()
	c.Start()
	return &Scheduler{core: c}
}

type Job struct {
	scheduler *Scheduler
	id        cron.EntryID
}

var _ scron.ScheduledEvent = (*Job)(nil)

func (j *Job) Cancel(ctx context.Context) error {
	j.scheduler.core.Remove(j.id)
	return nil
}
