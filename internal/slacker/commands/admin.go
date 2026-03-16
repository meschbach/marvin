package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
)

func HandleAdminHelp(ctx context.Context, deps *CommandsDependencies, msg string) error {
	deps.SecurityLogger.LogAdminAction(deps.Context.UserID(), "admin_help", "")

	adminHelp := "👑 **Admin Help**\n\n" +
		"Here are some admin commands you can use:\n\n" +
		"• `list pending requests` - See tool approval requests\n" +
		"• `approve tool <request-id>` - Approve a tool request\n" +
		"• `reject tool <request-id>` - Reject a tool request\n" +
		"• `model access list` - Show model access settings\n" +
		"• `allow model <model-name>` - Allow a model\n" +
		"• `deny model <model-name>` - Deny a model\n" +
		"• `admin help <topic>` - Get admin-specific help"

	return deps.MessageSender.SendMessage(ctx, deps.Context.UserID(), adminHelp)
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
