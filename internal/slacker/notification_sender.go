package slacker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

// NotificationSender sends Slack notifications
type NotificationSender struct {
	client     *slack.Client
	adminUsers []string
}

// NewNotificationSender creates a new notification sender
func NewNotificationSender(client *slack.Client, adminUsers []string) *NotificationSender {
	return &NotificationSender{
		client:     client,
		adminUsers: adminUsers,
	}
}

// GetClient returns the underlying Slack client
func (ns *NotificationSender) GetClient() *slack.Client {
	return ns.client
}

// NotifyAdmins sends approval notifications to admin users
func (ns *NotificationSender) NotifyAdmins(ctx context.Context, request *ToolApprovalRequest) error {
	message := ns.formatApprovalForSlack(request)

	for _, adminID := range ns.adminUsers {
		// Open DM channel with admin
		channel, _, _, err := ns.client.OpenConversationContext(ctx, &slack.OpenConversationParameters{
			Users: []string{adminID},
		})
		if err != nil {
			// Log error but continue with other admins
			continue
		}

		// Send approval request
		_, _, err = ns.client.PostMessageContext(
			ctx,
			channel.ID,
			slack.MsgOptionText(message, false),
		)
		if err != nil {
			// Log error but continue with other admins
			continue
		}
	}

	return nil
}

// SendApprovalNotification sends approval status notification to the original requester
func (ns *NotificationSender) SendApprovalNotification(ctx context.Context, requesterID, adminID, requestID, status, toolID, reason string) error {
	var message strings.Builder

	if status == "approved" {
		message.WriteString("✅ Your tool request has been approved!\n\n")
		fmt.Fprintf(&message, "• Request ID: %s\n", requestID)
		fmt.Fprintf(&message, "• Tool ID: %s\n", toolID)
		fmt.Fprintf(&message, "• Approved by: <@%s>\n", adminID)
		fmt.Fprintf(&message, "• Reason: %s\n\n", reason)
		message.WriteString("The tool is now available in your conversations. Try using it!")
	} else {
		message.WriteString("❌ Your tool request has been rejected.\n\n")
		fmt.Fprintf(&message, "• Request ID: %s\n", requestID)
		fmt.Fprintf(&message, "• Tool ID: %s\n", toolID)
		fmt.Fprintf(&message, "• Rejected by: <@%s>\n", adminID)
		fmt.Fprintf(&message, "• Reason: %s\n\n", reason)
		message.WriteString("If you believe this is an error, please contact an admin for review.")
	}

	// Open DM channel with requester (not admin)
	channel, _, _, err := ns.client.OpenConversationContext(ctx, &slack.OpenConversationParameters{
		Users: []string{requesterID},
	})
	if err != nil {
		return fmt.Errorf("opening DM channel: %w", err)
	}

	_, _, err = ns.client.PostMessageContext(
		ctx,
		channel.ID,
		slack.MsgOptionText(message.String(), false),
	)
	return err
}

// formatRequestID creates a unique request ID for admin notifications
func formatRequestID(requesterID string, timestamp time.Time) string {
	return fmt.Sprintf("%s-%s", requesterID, timestamp.Format("20060102-150405"))
}

// formatApprovalForSlack formats an approval request for Slack display
func (ns *NotificationSender) formatApprovalForSlack(request *ToolApprovalRequest) string {
	requestID := formatRequestID(request.RequesterID, request.Timestamp)
	var message strings.Builder

	message.WriteString("🔧 **Tool Approval Request**\n\n")
	fmt.Fprintf(&message, "• **Requester:** <@%s>\n", request.RequesterID)
	fmt.Fprintf(&message, "• **Tool Type:** %s\n", request.ToolType)
	fmt.Fprintf(&message, "• **Tool ID:** %s\n", request.ToolID)
	fmt.Fprintf(&message, "• **Request ID:** %s\n", requestID)
	fmt.Fprintf(&message, "• **Timestamp:** %s\n", request.Timestamp.Format("2006-01-02 15:04:05"))

	if request.Config != nil {
		message.WriteString("• **Configuration:**\n")
		fmt.Fprintf(&message, "```%+v```", request.Config)
	}

	message.WriteString("\n")
	message.WriteString("👉 Please review and approve/reject this request.\n\n")
	fmt.Fprintf(&message, "**To approve:** Reply with \"Approve %s\"\n", requestID)
	fmt.Fprintf(&message, "**To reject:** Reply with \"Reject %s because [reason]\"", requestID)

	return message.String()
}

// SendMessage sends a message to a user via DM
func (ns *NotificationSender) SendMessage(ctx context.Context, userID, message string) error {
	// Open DM channel
	channel, _, _, err := ns.client.OpenConversationContext(ctx, &slack.OpenConversationParameters{
		Users: []string{userID},
	})
	if err != nil {
		return fmt.Errorf("opening DM channel: %w", err)
	}

	// Send message
	_, _, err = ns.client.PostMessageContext(
		ctx,
		channel.ID,
		slack.MsgOptionText(message, false),
	)
	return err
}
