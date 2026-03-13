package cron

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/meschbach/go-junk-bucket/pkg/fx"
)

type mockCronJob struct {
	id         int64
	tab        string
	notify     Job
	cancelWith *MockCron
}

func (f *mockCronJob) Cancel(ctx context.Context) error {
	return f.cancelWith.cancelJob(ctx, f.id)
}

type MockCron struct {
	state     sync.Mutex
	jobIDS    atomic.Int64
	scheduled []*mockCronJob
}

func NewMockCron() *MockCron {
	return &MockCron{
		state:  sync.Mutex{},
		jobIDS: atomic.Int64{},
	}
}

var _ Scheduler = (*MockCron)(nil)

func (f *MockCron) Schedule(ctx context.Context, tabSpec string, notify Job) (ScheduledEvent, error) {
	id := f.jobIDS.Add(1)
	job := &mockCronJob{
		id:     id,
		tab:    tabSpec,
		notify: notify,
	}

	f.state.Lock()
	defer f.state.Unlock()
	f.scheduled = append(f.scheduled, job)
	return job, nil
}

func (f *MockCron) cancelJob(ctx context.Context, id int64) error {
	f.state.Lock()
	defer f.state.Unlock()

	f.scheduled = fx.Filter(f.scheduled, func(e *mockCronJob) bool {
		return e.id != id
	})
	return nil
}

func (f *MockCron) TickAll() {
	for _, t := range f.scheduled {
		t.notify.OnCron()
	}
}
