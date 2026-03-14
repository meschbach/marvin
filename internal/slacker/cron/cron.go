// Package cron provides a service to push a message to a specific agent at a repeated time
package cron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/meschbach/marvin/internal/junk"
	"github.com/meschbach/marvin/internal/slacker/storage"
)

type triggerState struct {
	id             string
	runtimeControl ScheduledEvent
	descriptor     *Trigger
	mediator       *Mediator
}

func (t *triggerState) OnCron() {
	t.mediator.trigger(t)
}

// Mediator provides a simplified interface for interacting with cron systems
type Mediator struct {
	scheduler  Scheduler
	dispatcher TriggerDispatcher
	storage    storage.User

	state    sync.Mutex
	started  bool
	triggers []*triggerState
}

type MediatorOpt func(mediator *Mediator)

func NewMediator(cron Scheduler, onTrigger TriggerDispatcher, storage storage.User, opts ...MediatorOpt) *Mediator {
	m := &Mediator{
		scheduler:  cron,
		dispatcher: onTrigger,
		storage:    storage,
		state:      sync.Mutex{},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Mediator) Register(ctx context.Context, forUser storage.UserKey, t *Trigger) (id string, err error) {
	v7, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	id = v7.String()

	state := &triggerState{
		id:             id,
		runtimeControl: nil,
		descriptor:     t,
		mediator:       m,
	}
	(func() {
		m.state.Lock()
		defer m.state.Unlock()
		m.triggers = append(m.triggers, state)
	})()

	state.runtimeControl, err = m.scheduler.Schedule(ctx, t.Spec, state)
	if err != nil {
		return id, err
	}

	if t.Source != "config" {
		s := &persistenceLayer{storage: m.storage}
		if err := s.appendTrigger(ctx, forUser, id, t); err != nil {
			return id, err
		}
	}

	//
	return id, nil
}

func (m *Mediator) trigger(t *triggerState) {
	ctx, span := tracer.Start(context.Background(), "cron.Trigger")
	defer span.End()

	fmt.Printf("Dispatching....")
	if err := m.dispatcher.OnTrigger(ctx, *t.descriptor); err != nil {
		junk.RecordSpanErrorNoLint(span, err)
	}
}

func (m *Mediator) Shutdown(ctx context.Context) error {
	return nil
}

func (m *Mediator) Start(ctx context.Context) error {
	m.state.Lock()
	defer m.state.Unlock()
	return m.startedLocked(ctx)
}

func (m *Mediator) startedLocked(ctx context.Context) error {
	users, err := m.storage.ListUsers(ctx)
	if err != nil {
		return err
	}
	var problems []error
	for userKey, err := range users {
		if err != nil {
			problems = append(problems, err)
		} else {
			alarmsJSON, err := m.storage.GetUserKey(ctx, userKey, "alarms.json")
			if err != nil {
				problems = append(problems, err)
			} else {
				var file UserPersistedAlarmsFile
				if err := json.Unmarshal([]byte(alarmsJSON), &file); err != nil {
					problems = append(problems, err)
				} else {
					for _, alarm := range file.Alarms {
						if alarm.Source == "" {
							fmt.Printf("Removing legacy alarm %s during migration\n", alarm.ID)
							persister := &persistenceLayer{storage: m.storage}
							if err := persister.deleteTrigger(ctx, userKey, alarm.ID); err != nil {
								problems = append(problems, err)
							}
							continue
						}

						if alarm.Source == "config" {
							fmt.Printf("Skipping ephemeral config alarm %s (should not be in storage)\n", alarm.ID)
							continue
						}

						fmt.Printf("Activating alarm %s", alarm.ID)
						state := &triggerState{
							id:             alarm.ID,
							runtimeControl: nil,
							descriptor: &Trigger{
								Spec:    alarm.Spec,
								Target:  alarm.Target,
								Message: alarm.Message,
								Source:  alarm.Source,
							},
							mediator: m,
						}
						state.runtimeControl, err = m.scheduler.Schedule(ctx, alarm.Spec, state)
						m.triggers = append(m.triggers, state)
					}
				}
			}
		}
	}
	out := errors.Join(problems...)
	if out == nil {
		m.started = true
	}
	return out
}
