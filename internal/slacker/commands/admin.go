package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
)

func HandleAdminHelp(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func HandleEscalate(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func SendMessage(client *slack.Client, channelID, message string) error {
	_, _, err := client.PostMessage(
		channelID,
		slack.MsgOptionText(message, false),
	)
	return err
}

func FormatHelpResponse(commands map[string]string) string {
	var response strings.Builder
	response.WriteString("Available Commands:\n\n")

	for cmd, desc := range commands {
		fmt.Fprintf(&response, "  %s - %s\n", cmd, desc)
	}

	return response.String()
}
