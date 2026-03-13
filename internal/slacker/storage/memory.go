package storage

import (
	"context"
	"iter"
	"maps"
	"sync"
)

type memoryUserKVStorage struct {
	state  sync.Mutex
	values map[string]string
}

type MemoryUser struct {
	state sync.Mutex

	users map[UserKey]*memoryUserKVStorage
}

func NewMemoryUser() *MemoryUser {
	return &MemoryUser{
		state: sync.Mutex{},
		users: make(map[UserKey]*memoryUserKVStorage),
	}
}

var _ User = (*MemoryUser)(nil)

func (m *MemoryUser) getUser(ctx context.Context, user UserKey) *memoryUserKVStorage {
	m.state.Lock()
	defer m.state.Unlock()

	store, has := m.users[user]
	if !has {
		store = &memoryUserKVStorage{
			state:  sync.Mutex{},
			values: make(map[string]string),
		}
		m.users[user] = store
	}
	return store
}

func (m *MemoryUser) GetUserKey(ctx context.Context, user UserKey, key string) (string, error) {
	store := m.getUser(ctx, user)

	store.state.Lock()
	defer store.state.Unlock()
	return store.values[key], nil
}

func (m *MemoryUser) PutUserKey(ctx context.Context, user UserKey, key, content string) error {
	store := m.getUser(ctx, user)

	store.state.Lock()
	defer store.state.Unlock()

	store.values[key] = content
	return nil
}

func (m *MemoryUser) ListUsers(ctx context.Context) (iter.Seq2[UserKey, error], error) {
	m.state.Lock()
	defer m.state.Unlock()

	keys := maps.Keys(m.users)
	return func(yield func(UserKey, error) bool) {
		for key := range keys {
			if !yield(key, nil) {
				return
			}
		}
	}, nil
}
