package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/slacker"
	"github.com/slack-go/slack"
)

type CommandsDependencies struct {
	ChannelID   string
	ToolManager interface {
		HandleToolIntent(ctx context.Context, slackCtx *slacker.SlackContext, session *slacker.UserSession, intent *slacker.ToolManagementIntent) error
		HandleListTools(ctx context.Context, slackCtx *slacker.SlackContext, session *slacker.UserSession) error
		ResetSession(ctx context.Context, slackCtx *slacker.SlackContext, session *slacker.UserSession) error
	}
	SessionManager interface{}
	SecurityLogger interface{}
	SlackClient    *slack.Client
	Config         *config.File
	Connection     *slacker.SlackConnection
	MessageHandler interface {
		HandleModelAccess(ctx context.Context, updater *slacker.SlackUpdater) error
		HandleAdminHelp(ctx context.Context, updater *slacker.SlackUpdater) error
		HandleEscalate(ctx context.Context, updater *slacker.SlackUpdater) error
	}
}

func handleHelp(ctx context.Context, deps *CommandsDependencies, msg string) error {
	helpText := RenderHelp(nil)
	return SendMessage(deps.SlackClient, deps.ChannelID, helpText)
}

func handleTools(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func handleThinking(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func handlePreferences(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func handleResetSession(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func handleAddTool(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func handleListTools(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func handleRemoveTool(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func handleApprove(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func handleReject(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func handleModelAccess(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func handleAdminHelp(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func handleEscalate(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

type MessageSender interface {
	SendMessage(ctx context.Context, userID, message string) error
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
