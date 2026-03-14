package cron

import "context"

type Trigger struct {
	Spec    string
	Target  []string
	Message string
	Source  string
}

type TriggerDispatcher interface {
	OnTrigger(ctx context.Context, t Trigger) error
}
