package slacker

import (
	"context"

	"github.com/slack-go/slack"
)

// SlackClientAPI abstracts Slack API operations for testing.
// This enables mock implementations while maintaining a consistent interface.
type SlackClientAPI interface {
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
	PostMessage(channelID string, options ...slack.MsgOption) (string, string, error)
	GetUserInfo(userID string) (*slack.User, error)
	AuthTest() (*slack.AuthTestResponse, error)
	OpenConversation(context.Context, *slack.OpenConversationParameters) (*slack.Channel, bool, bool, error)
	UpdateMessageContext(ctx context.Context, channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error)
}

// slackClientAdapter wraps *slack.Client to satisfy SlackClientAPI
// by adapting method signatures.
type slackClientAdapter struct {
	client *slack.Client
}

func (a *slackClientAdapter) PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error) {
	return a.client.PostMessageContext(ctx, channelID, options...)
}

func (a *slackClientAdapter) PostMessage(channelID string, options ...slack.MsgOption) (string, string, error) {
	return a.client.PostMessage(channelID, options...)
}

func (a *slackClientAdapter) GetUserInfo(userID string) (*slack.User, error) {
	return a.client.GetUserInfo(userID)
}

func (a *slackClientAdapter) AuthTest() (*slack.AuthTestResponse, error) {
	return a.client.AuthTest()
}

func (a *slackClientAdapter) OpenConversation(ctx context.Context, params *slack.OpenConversationParameters) (*slack.Channel, bool, bool, error) {
	return a.client.OpenConversation(params)
}

func (a *slackClientAdapter) UpdateMessageContext(ctx context.Context, channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
	return a.client.UpdateMessageContext(ctx, channelID, timestamp, options...)
}
