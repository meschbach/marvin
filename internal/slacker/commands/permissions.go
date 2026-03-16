package commands

import (
	"fmt"

	"github.com/slack-go/slack"
)

func IsAdmin(userID string, admins []string) bool {
	for _, admin := range admins {
		if userID == admin {
			return true
		}
	}
	return false
}

func SendPermissionDenied(client *slack.Client, channelID, userID string) error {
	_, _, err := client.PostMessage(
		channelID,
		slack.MsgOptionText(
			fmt.Sprintf("<@%s> You don't have permission to run this command.", userID),
			false,
		),
	)
	return err
}
