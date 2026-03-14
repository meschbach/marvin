package cron

import (
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/meschbach/marvin/internal/slacker/storage"
	"github.com/meschbach/marvin/internal/slacker/storage/storagetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type JigOpt func(j *Jig)

type Jig struct {
	Cron       *MockCron
	Dispatcher *CapturingDispatcher
	Storage    storage.User
	Mediator   *Mediator
}

func NewJig(opts ...JigOpt) *Jig {
	j := &Jig{}
	for _, opt := range opts {
		opt(j)
	}
	if j.Cron == nil {
		j.Cron = NewMockCron()
	}
	if j.Dispatcher == nil {
		j.Dispatcher = NewCapturingDispatcher()
	}
	if j.Storage == nil {
		j.Storage = storage.NewMemoryUser()
	}
	j.Mediator = NewMediator(j.Cron, j.Dispatcher, j.Storage)
	return j
}

func WithStorage(store storage.User) JigOpt {
	return func(j *Jig) {
		j.Storage = store
	}
}

func TestCronTrigger(t *testing.T) {
	t.Parallel()

	userKey := storagetest.FakeUserKey()
	message := faker.Sentence()
	target := []string{faker.Word(), faker.Word(), faker.Word()}
	ctx := t.Context()

	jig := NewJig()
	c := jig.Mediator
	id, err := c.Register(ctx, userKey, &Trigger{
		Spec:    "* * * * *",
		Target:  target,
		Message: message,
		Source:  "user",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	jig.Cron.TickAll()
	if assert.Len(t, jig.Dispatcher.Triggers, 1) {
		trigger := jig.Dispatcher.Triggers[0]
		assert.Equal(t, message, trigger.Message)
		assert.Equal(t, target, trigger.Target)
	}
}

func TestCronStoreAndReactivate_IDIsPreserved(t *testing.T) {
	t.Parallel()

	forUser := storagetest.FakeUserKey()
	ctx := t.Context()

	firstJig := NewJig()
	registeringCron := firstJig.Mediator
	originalID, err := registeringCron.Register(ctx, forUser, &Trigger{
		Spec:    "* * * * *",
		Target:  []string{faker.Word()},
		Message: faker.Sentence(),
		Source:  "user",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, originalID)

	require.NoError(t, registeringCron.Shutdown(ctx))

	secondJig := NewJig(WithStorage(firstJig.Storage))
	require.NoError(t, secondJig.Mediator.Start(t.Context()))

	require.Len(t, secondJig.Mediator.triggers, 1)
	assert.Equal(t, originalID, secondJig.Mediator.triggers[0].id, "recovered trigger should have the same ID as the original")
}

func TestCronStoreAndReactivate(t *testing.T) {
	t.Parallel()

	forUser := storagetest.FakeUserKey()
	message := faker.Sentence()
	target := []string{faker.Word(), faker.Word(), faker.Word()}
	ctx := t.Context()

	firstJig := NewJig()
	registeringCron := firstJig.Mediator
	id, err := registeringCron.Register(ctx, forUser, &Trigger{
		Spec:    "* * * * *",
		Target:  target,
		Message: message,
		Source:  "user",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	require.NoError(t, registeringCron.Shutdown(ctx))

	secondJig := NewJig(WithStorage(firstJig.Storage))
	require.NoError(t, secondJig.Mediator.Start(t.Context()))
	secondJig.Cron.TickAll()

	assert.Len(t, firstJig.Dispatcher.Triggers, 0, "first jig should not have been triggered")

	if assert.Len(t, secondJig.Dispatcher.Triggers, 1) {
		trigger := secondJig.Dispatcher.Triggers[0]
		assert.Equal(t, message, trigger.Message)
		assert.Equal(t, target, trigger.Target)
	}
}

func TestCronConfigTrigger_NotPersisted(t *testing.T) {
	t.Parallel()

	userKey := storagetest.FakeUserKey()
	ctx := t.Context()

	jig := NewJig()
	_, err := jig.Mediator.Register(ctx, userKey, &Trigger{
		Spec:    "* * * * *",
		Target:  []string{faker.Word()},
		Message: faker.Sentence(),
		Source:  "config",
	})
	require.NoError(t, err)

	alarmsJSON, err := jig.Storage.GetUserKey(ctx, userKey, "alarms.json")
	require.NoError(t, err)
	assert.Empty(t, alarmsJSON, "config triggers should not be persisted")
}

func TestCronUserTrigger_IsPersisted(t *testing.T) {
	t.Parallel()

	userKey := storagetest.FakeUserKey()
	ctx := t.Context()

	jig := NewJig()
	_, err := jig.Mediator.Register(ctx, userKey, &Trigger{
		Spec:    "* * * * *",
		Target:  []string{faker.Word()},
		Message: faker.Sentence(),
		Source:  "user",
	})
	require.NoError(t, err)

	alarmsJSON, err := jig.Storage.GetUserKey(ctx, userKey, "alarms.json")
	require.NoError(t, err)
	assert.NotEmpty(t, alarmsJSON, "user triggers should be persisted")
}
