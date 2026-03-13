package cron

import (
	"context"
	"sync"
)

type CapturingDispatcher struct {
	state    sync.Mutex
	Triggers []Trigger
}

func NewCapturingDispatcher() *CapturingDispatcher {
	return &CapturingDispatcher{}
}

func (d *CapturingDispatcher) OnTrigger(ctx context.Context, t Trigger) error {
	d.state.Lock()
	defer d.state.Unlock()

	d.Triggers = append(d.Triggers, t)
	return nil
}
