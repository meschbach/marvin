package cron

type UserPersistedAlarmsFile struct {
	Version int                        `json:"version"`
	Alarms  []UserPersistedAlarmStanza `json:"alarms"`
}

type UserPersistedAlarmStanza struct {
	ID      string   `json:"id"`
	Spec    string   `json:"spec"`
	Target  []string `json:"target"`
	Message string   `json:"message"`
	Source  string   `json:"source"`
}
