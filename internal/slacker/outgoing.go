package slacker

import (
	"context"

	"github.com/slack-go/slack"
)

// OutgoingMessages defines the interface for notification senders
type OutgoingMessages interface {
	SendMessage(ctx context.Context, userID, message string) error
	NotifyAdmins(ctx context.Context, request *ToolApprovalRequest) error
	SendApprovalNotification(ctx context.Context, requesterID, adminID, requestID, status, toolID, reason string) error
	GetClient() *slack.Client
}
