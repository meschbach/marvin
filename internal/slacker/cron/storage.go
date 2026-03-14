package cron

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/meschbach/marvin/internal/slacker/storage"
)

const alarmsFile = "alarms.json"

type persistenceLayer struct {
	storage storage.User
}

func (s *persistenceLayer) appendTrigger(ctx context.Context, forUser storage.UserKey, id string, trigger *Trigger) error {
	content, err := s.storage.GetUserKey(ctx, forUser, alarmsFile)
	if err != nil {
		return err
	}
	var file UserPersistedAlarmsFile
	if content != "" {
		if err := json.Unmarshal([]byte(content), &file); err != nil {
			return fmt.Errorf("failed to unmarshal alarms file: %w", err)
		}
	} else {
		file.Version = 1
	}

	file.Alarms = append(file.Alarms, UserPersistedAlarmStanza{
		ID:      id,
		Spec:    trigger.Spec,
		Target:  trigger.Target,
		Message: trigger.Message,
		Source:  trigger.Source,
	})

	fileBytes, err := json.Marshal(file)
	if err != nil {
		return fmt.Errorf("failed to marshal alarms file: %w", err)
	}

	return s.storage.PutUserKey(ctx, forUser, alarmsFile, string(fileBytes))
}

func (s *persistenceLayer) deleteTrigger(ctx context.Context, forUser storage.UserKey, id string) error {
	content, err := s.storage.GetUserKey(ctx, forUser, alarmsFile)
	if err != nil {
		return err
	}
	var file UserPersistedAlarmsFile
	if content != "" {
		if err := json.Unmarshal([]byte(content), &file); err != nil {
			return fmt.Errorf("failed to unmarshal alarms file: %w", err)
		}
	} else {
		return nil
	}

	filtered := make([]UserPersistedAlarmStanza, 0, len(file.Alarms))
	for _, alarm := range file.Alarms {
		if alarm.ID != id {
			filtered = append(filtered, alarm)
		}
	}
	file.Alarms = filtered

	fileBytes, err := json.Marshal(file)
	if err != nil {
		return fmt.Errorf("failed to marshal alarms file: %w", err)
	}

	return s.storage.PutUserKey(ctx, forUser, alarmsFile, string(fileBytes))
}
