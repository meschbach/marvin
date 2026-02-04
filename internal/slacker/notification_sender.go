package slacker

import (
	"fmt"
	"strings"

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

// NotifyAdmins sends approval notifications to admin users
func (ns *NotificationSender) NotifyAdmins(request *ToolApprovalRequest) error {
	message := ns.formatApprovalForSlack(request)

	for _, adminID := range ns.adminUsers {
		// Open DM channel with admin
		channel, _, _, err := ns.client.OpenConversation(&slack.OpenConversationParameters{
			Users: []string{adminID},
		})
		if err != nil {
			// Log error but continue with other admins
			continue
		}

		// Send approval request
		_, _, err = ns.client.PostMessage(
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

// SendApprovalNotification sends approval status notification
func (ns *NotificationSender) SendApprovalNotification(adminID, requestID, status string) error {
	// This would need approval details from the approval workflow
	// For now, creating a basic notification
	message := fmt.Sprintf("🔧 Tool request %s has been **%s** by <@%s>", requestID, status, adminID)

	// Open DM channel with requester (would need requester ID from workflow)
	// For now, sending to admin who approved/rejected
	channel, _, _, err := ns.client.OpenConversation(&slack.OpenConversationParameters{
		Users: []string{adminID},
	})
	if err != nil {
		return fmt.Errorf("opening DM channel: %w", err)
	}

	_, _, err = ns.client.PostMessage(
		channel.ID,
		slack.MsgOptionText(message, false),
	)
	return err
}

// formatApprovalForSlack formats an approval request for Slack display
func (ns *NotificationSender) formatApprovalForSlack(request *ToolApprovalRequest) string {
	var message strings.Builder

	message.WriteString("🔧 **Tool Approval Request**\n\n")
	message.WriteString(fmt.Sprintf("• **Requester:** <@%s>\n", request.RequesterID))
	message.WriteString(fmt.Sprintf("• **Tool Type:** %s\n", request.ToolType))
	message.WriteString(fmt.Sprintf("• **Tool ID:** %s\n", request.ToolID))
	message.WriteString(fmt.Sprintf("• **Timestamp:** %s\n", request.Timestamp.Format("2006-01-02 15:04:05")))

	if request.Config != nil {
		message.WriteString("• **Configuration:**\n")
		message.WriteString(fmt.Sprintf("```%+v```", request.Config))
	}

	message.WriteString("\n")
	message.WriteString("👉 Please review and approve/reject this request.")

	return message.String()
}

// SendMessage sends a message to a user via DM
func (ns *NotificationSender) SendMessage(userID, message string) error {
	// Open DM channel
	channel, _, _, err := ns.client.OpenConversation(&slack.OpenConversationParameters{
		Users: []string{userID},
	})
	if err != nil {
		return fmt.Errorf("opening DM channel: %w", err)
	}

	// Send message
	_, _, err = ns.client.PostMessage(
		channel.ID,
		slack.MsgOptionText(message, false),
	)
	return err
}
