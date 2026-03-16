package commands

import (
	"context"
	"fmt"
	"strings"
)

func HandleApprove(ctx context.Context, deps *CommandsDependencies, msg string) error {
	if !deps.ApprovalWorkflow.IsAdmin(deps.Context.UserID()) {
		return send(ctx, deps, deps.Context.UserID(), "", "❌ Only admins can approve/reject tool requests")
	}

	parts := strings.Fields(msg)
	if len(parts) < 2 {
		return send(ctx, deps, deps.Context.UserID(), "", "❌ Please specify a request ID")
	}

	requestID := parts[1]
	if err := deps.ApprovalWorkflow.ApproveTool(ctx, deps.Context.UserID(), requestID, "Approved via command"); err != nil {
		return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("❌ Error approving request: %s", err.Error()))
	}

	return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("✅ Tool request %s approved", requestID))
}

func HandleReject(ctx context.Context, deps *CommandsDependencies, msg string) error {
	if !deps.ApprovalWorkflow.IsAdmin(deps.Context.UserID()) {
		return send(ctx, deps, deps.Context.UserID(), "", "❌ Only admins can approve/reject tool requests")
	}

	parts := strings.Fields(msg)
	if len(parts) < 2 {
		return send(ctx, deps, deps.Context.UserID(), "", "❌ Please specify a request ID")
	}

	requestID := parts[1]
	reason := "No reason provided"
	if len(parts) > 2 {
		reason = strings.Join(parts[2:], " ")
	}

	if err := deps.ApprovalWorkflow.RejectTool(ctx, deps.Context.UserID(), requestID, reason); err != nil {
		return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("❌ Error rejecting request: %s", err.Error()))
	}

	return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("✅ Tool request %s rejected", requestID))
}

func HandleShareTool(ctx context.Context, deps *CommandsDependencies, msg string) error {
	parts := strings.Fields(msg)
	if len(parts) < 4 {
		return send(ctx, deps, deps.Context.UserID(), "", "❌ Please specify a tool and user. Example: share tool my-tool with @username")
	}

	if parts[0] != "tool" {
		return send(ctx, deps, deps.Context.UserID(), "", "❌ Please use format: share tool <tool-name> with @username")
	}
	toolName := parts[1]

	if parts[2] != "with" {
		return send(ctx, deps, deps.Context.UserID(), "", "❌ Please use format: share tool <tool-name> with @username")
	}
	targetUser := strings.TrimPrefix(parts[3], "@")

	session, err := deps.SessionManager.GetOrCreateSession(ctx, deps.Context.UserID(), deps.Context.ChannelID(), nil)
	if err != nil {
		return send(ctx, deps, deps.Context.UserID(), "", "❌ Error retrieving session")
	}

	userTools := session.AvailableTools
	found := false
	for _, t := range userTools {
		if t == toolName {
			found = true
			break
		}
	}

	if !found {
		return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("❌ Tool %q not found in your available tools", toolName))
	}

	if err := deps.ToolSet.ShareTool(ctx, toolName, targetUser); err != nil {
		deps.SecurityLogger.LogError(deps.Context.UserID(), "share_tool", fmt.Sprintf("failed to share tool: %v", err))
		return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("❌ Error sharing tool: %s", err.Error()))
	}

	deps.SecurityLogger.LogToolShare(deps.Context.UserID(), targetUser, toolName)
	return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("✅ Tool %q has been shared with user %s", toolName, targetUser))
}
