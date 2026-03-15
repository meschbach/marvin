package storage

import (
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStorage(t *testing.T) {
	t.Parallel()
	exampleUser := UserKey{
		UserID:  faker.Word(),
		Channel: faker.Word(),
	}
	exampleKey := faker.Word() + "/" + faker.Word()
	content := faker.Sentence()

	m := NewMemoryUser()
	require.NoError(t, m.PutUserKey(t.Context(), exampleUser, exampleKey, content))
	givenValue, err := m.GetUserKey(t.Context(), exampleUser, exampleKey)
	require.NoError(t, err)
	assert.Equal(t, content, givenValue)
}
