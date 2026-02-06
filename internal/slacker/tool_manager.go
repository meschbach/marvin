package slacker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
)

// ToolManagerImpl handles tool management operations
type ToolManagerImpl struct {
	approvalWorkflow   *ApprovalWorkflow
	tenantToolSet      *query.TenantToolSet
	securityLogger     *sec.SecurityLogger
	notificationSender OutgoingMessages
	sessionManager     *SessionManager
}

// NewToolManager creates a new tool manager
func NewToolManager(
	approvalWorkflow *ApprovalWorkflow,
	tenantToolSet *query.TenantToolSet,
	securityLogger *sec.SecurityLogger,
	notificationSender OutgoingMessages,
	sessionManager *SessionManager,
) *ToolManagerImpl {
	return &ToolManagerImpl{
		approvalWorkflow:   approvalWorkflow,
		tenantToolSet:      tenantToolSet,
		securityLogger:     securityLogger,
		notificationSender: notificationSender,
		sessionManager:     sessionManager,
	}
}

// HandleToolIntent processes tool management intents
func (tm *ToolManagerImpl) HandleToolIntent(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	switch intent.Action {
	case "add_tool":
		return tm.handleAddTool(ctx, slackCtx, session, intent)
	case "share_tool":
		return tm.handleShareTool(ctx, slackCtx, session, intent)
	case "list_tools":
		return tm.handleListTools(ctx, slackCtx, session)
	case "remove_tool":
		return tm.handleRemoveTool(ctx, slackCtx, session, intent)
	case "approve_tool":
		return tm.handleApprovalCommand(ctx, slackCtx, session, intent, "approve")
	case "reject_tool":
		return tm.handleApprovalCommand(ctx, slackCtx, session, intent, "reject")
	case "reset_session":
		return tm.handleResetSession(ctx, slackCtx, session)
	default:
		return tm.notificationSender.SendMessage(slackCtx.UserID, fmt.Sprintf("I don't know how to handle: %s", intent.Action))
	}
}

// handleAddTool handles adding new tools
func (tm *ToolManagerImpl) handleAddTool(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	// Parse tool configuration
	toolConfig, err := ParseToolConfig(intent.ToolType, intent.Config.(string))
	if err != nil {
		return tm.notificationSender.SendMessage(slackCtx.UserID, fmt.Sprintf("❌ Error parsing tool configuration: %s", err.Error()))
	}

	// Check if approval is needed
	if config.RequiresApproval(intent.ToolType) {
		// Submit for approval
		request := &ToolApprovalRequest{
			ToolID:        GenerateToolID(slackCtx.UserID, intent.ToolType, getNameFromConfig(toolConfig)),
			RequesterID:   slackCtx.UserID,
			ToolType:      intent.ToolType,
			Config:        toolConfig,
			RequesterName: slackCtx.UserName,
			Timestamp:     time.Now(),
		}

		requestID, err := tm.approvalWorkflow.RequestToolApproval(request)
		if err != nil {
			return tm.notificationSender.SendMessage(slackCtx.UserID, fmt.Sprintf("❌ Error submitting approval request: %s", err.Error()))
		}

		tm.securityLogger.LogToolRequest(slackCtx.UserID, intent.ToolType, fmt.Sprintf("%+v", toolConfig))

		return tm.notificationSender.SendMessage(slackCtx.UserID, fmt.Sprintf("📋 Tool approval request submitted:\n• Tool ID: %s\n• Status: Pending approval\n• I'll notify you when it's approved.", requestID))
	} else {
		// HTTP tools can be added directly
		toolID := GenerateToolID(slackCtx.UserID, intent.ToolType, getNameFromConfig(toolConfig))

		// TODO: Add tool directly to user's tool set
		// This would require extending the TenantToolSet to support dynamic tool addition

		tm.securityLogger.LogToolAdded(slackCtx.UserID, toolID, intent.ToolType)

		return tm.notificationSender.SendMessage(slackCtx.UserID, fmt.Sprintf("✅ Added %s tool successfully. You can now use it in your conversations.", intent.ToolType))
	}
}

// handleShareTool handles sharing tools with other users
func (tm *ToolManagerImpl) handleShareTool(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	// TODO: Implement tool sharing logic
	tm.securityLogger.LogToolShare(slackCtx.UserID, intent.TargetUser, intent.Target)
	return tm.notificationSender.SendMessage(slackCtx.UserID, fmt.Sprintf("🔄 Tool sharing requested. Feature coming soon!"))
}

// handleListTools lists available tools for the user
func (tm *ToolManagerImpl) handleListTools(ctx context.Context, slackCtx *SlackContext, session *UserSession) error {
	tools := session.GetAvailableTools()

	if len(tools) == 0 {
		return tm.notificationSender.SendMessage(slackCtx.UserID, "You don't have any tools available yet. Try adding an HTTP MCP tool!")
	}

	var toolList strings.Builder
	toolList.WriteString("🔧 **Your Available Tools:**\n\n")

	for i, tool := range tools {
		toolList.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, tool))
	}

	return tm.notificationSender.SendMessage(slackCtx.UserID, toolList.String())
}

// handleRemoveTool handles removing tools
func (tm *ToolManagerImpl) handleRemoveTool(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	// TODO: Implement tool removal logic
	return tm.notificationSender.SendMessage(slackCtx.UserID, fmt.Sprintf("🗑️ Tool removal requested. Feature coming soon!"))
}

// handleApprovalCommand handles approve/reject commands via natural language
func (tm *ToolManagerImpl) handleApprovalCommand(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent, action string) error {
	// Verify admin permissions
	if !tm.approvalWorkflow.IsAdmin(slackCtx.UserID) {
		return tm.notificationSender.SendMessage(slackCtx.UserID, "❌ Only admins can approve/reject tool requests")
	}

	requestID := intent.Target
	if requestID == "" {
		return tm.notificationSender.SendMessage(slackCtx.UserID, "❌ Please specify a request ID")
	}

	if action == "approve" {
		if err := tm.approvalWorkflow.ApproveTool(slackCtx.UserID, requestID, "Approved via natural language"); err != nil {
			return tm.notificationSender.SendMessage(slackCtx.UserID, fmt.Sprintf("❌ Error approving request: %s", err.Error()))
		}
		// Send approval notification via notification sender
		return tm.notificationSender.SendMessage(slackCtx.UserID, fmt.Sprintf("✅ Tool request %s approved", requestID))
	} else {
		reason := "No reason provided"
		if intent.Config != nil {
			reason = intent.Config.(string)
		}
		if err := tm.approvalWorkflow.RejectTool(slackCtx.UserID, requestID, reason); err != nil {
			return tm.notificationSender.SendMessage(slackCtx.UserID, fmt.Sprintf("❌ Error rejecting request: %s", err.Error()))
		}
		// Send rejection notification via notification sender
		return tm.notificationSender.SendMessage(slackCtx.UserID, fmt.Sprintf("❌ Tool request %s rejected", requestID))
	}
}

// handleResetSession handles resetting user session context
func (tm *ToolManagerImpl) handleResetSession(ctx context.Context, slackCtx *SlackContext, session *UserSession) error {
	if err := tm.sessionManager.ClearSession(slackCtx.UserID, slackCtx.ChannelID); err != nil {
		return tm.notificationSender.SendMessage(slackCtx.UserID, fmt.Sprintf("❌ Error resetting session: %s", err.Error()))
	}

	tm.securityLogger.LogSessionEvent(slackCtx.UserID, slackCtx.ChannelID, "Session reset by user")
	return tm.notificationSender.SendMessage(slackCtx.UserID, "✅ Your conversation history has been cleared")
}
