package storage

import (
	"context"
	"iter"
)

// UserKey is the coordinates to identify a specific user
type UserKey struct {
	UserID  string
	Channel string
}

// User provides key value storage per user
type User interface {
	GetUserKey(ctx context.Context, user UserKey, key string) (string, error)
	PutUserKey(ctx context.Context, user UserKey, key, content string) error

	ListUsers(ctx context.Context) (iter.Seq2[UserKey, error], error)
}
