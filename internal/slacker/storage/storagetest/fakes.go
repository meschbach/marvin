package storagetest

import (
	"github.com/go-faker/faker/v4"
	"github.com/meschbach/marvin/internal/slacker/storage"
)

func FakeUserKey() storage.UserKey {
	return storage.UserKey{
		UserID:  faker.Word(),
		Channel: faker.Word(),
	}
}
