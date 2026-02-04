package slacker

import (
	"fmt"
	"time"
)

// ApprovalFormatter handles Slack formatting for approval requests
type ApprovalFormatter struct{}

// NewApprovalFormatter creates a new approval formatter
func NewApprovalFormatter() *ApprovalFormatter {
	return &ApprovalFormatter{}
}

// FormatApprovalForSlack formats an approval request for Slack notification
func (af *ApprovalFormatter) FormatApprovalForSlack(request *ToolApprovalRequest) string {
	requestID := generateRequestID(request.RequesterID, request.Timestamp)

	message := fmt.Sprintf("🔧 **Tool Approval Required**\n\n"+
		"**Requester:** <@%s>\n"+
		"**Tool Type:** %s\n"+
		"**Tool Name:** %s\n"+
		"**Request ID:** %s\n"+
		"**Requested:** %s\n\n"+
		"**Configuration:**\n```json\n%s\n```\n\n"+
		"**To approve:** Reply with \"Approve %s\"\n"+
		"**To reject:** Reply with \"Reject %s because [reason]\"",
		request.RequesterID,
		request.ToolType,
		request.ToolID,
		requestID,
		request.Timestamp.Format("2006-01-02 15:04:05"),
		af.formatConfigForSlack(request.Config),
		requestID,
		requestID,
	)

	return message
}

// formatConfigForSlack formats tool configuration for Slack display
func (af *ApprovalFormatter) formatConfigForSlack(config interface{}) string {
	return fmt.Sprintf("%+v", config)
}

// generateRequestID generates a unique request ID
func generateRequestID(requesterID string, timestamp time.Time) string {
	return requesterID[:8] + "-" + fmt.Sprintf("%d", timestamp.Unix())
}
