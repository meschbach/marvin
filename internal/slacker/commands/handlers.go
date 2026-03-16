package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	"github.com/slack-go/slack"
)

func HandleHelp(ctx context.Context, deps *CommandsDependencies, msg string) error {
	helpText := RenderHelp(nil)
	return send(ctx, deps, "", deps.Context.ChannelID(), helpText)
}

func HandleTools(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func HandleThinking(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func HandlePreferences(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

func HandleResetSession(ctx context.Context, deps *CommandsDependencies, msg string) error {
	if err := deps.SessionManager.ClearSession(deps.Context.UserID(), deps.Context.ChannelID()); err != nil {
		deps.SecurityLogger.LogError(deps.Context.UserID(), "session", fmt.Sprintf("Failed to reset session: %v", err))
		return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("❌ Error resetting session: %v", err))
	}

	deps.SecurityLogger.LogSessionEvent(deps.Context.UserID(), deps.Context.ChannelID(), "Session reset by user")
	return send(ctx, deps, "", deps.Context.ChannelID(), "✅ Your conversation history has been cleared")
}

func HandleAddTool(ctx context.Context, deps *CommandsDependencies, msg string) error {
	parts := strings.Fields(strings.ToLower(msg))
	toolType := ""
	configStr := ""

	for i, part := range parts {
		if part == "http" && i+1 < len(parts) {
			toolType = "http"
			if i+2 < len(parts) {
				configStr = strings.Join(parts[i+2:], " ")
			}
			break
		}
	}

	if toolType == "" {
		return send(ctx, deps, deps.Context.UserID(), "", "To add a tool, please use natural language like: \"add an HTTP MCP tool with config...\"")
	}

	toolConfig, err := deps.ToolParser.ParseToolConfig(toolType, configStr)
	if err != nil {
		deps.SecurityLogger.LogError(deps.Context.UserID(), "add_tool", fmt.Sprintf("Failed to parse tool config: %v", err))
		return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("❌ Error parsing tool configuration: %s", err.Error()))
	}

	if config.RequiresApproval(toolType) {
		configMap, _ := toolConfig.(map[string]interface{})
		request := &ToolApprovalRequest{
			ToolID:        deps.ToolParser.GenerateToolID(deps.Context.UserID(), toolType, extractNameFromConfig(configMap)),
			RequesterID:   deps.Context.UserID(),
			ToolType:      toolType,
			Config:        toolConfig,
			RequesterName: deps.Context.UserName(),
		}

		requestID, err := deps.ApprovalWorkflow.RequestToolApproval(ctx, request)
		if err != nil {
			return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("❌ Error submitting approval request: %s", err.Error()))
		}

		deps.SecurityLogger.LogToolRequest(deps.Context.UserID(), toolType, fmt.Sprintf("%+v", toolConfig))
		return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("📋 Tool approval request submitted:\n• Tool ID: %s\n• Status: Pending approval\n• I'll notify you when it's approved.", requestID))
	}

	configMap, _ := toolConfig.(map[string]interface{})
	toolID := deps.ToolParser.GenerateToolID(deps.Context.UserID(), toolType, extractNameFromConfig(configMap))
	deps.SecurityLogger.LogToolAdded(deps.Context.UserID(), toolID, toolType)
	return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("✅ Added %s tool successfully. You can now use it in your conversations.", toolType))
}

func HandleListTools(ctx context.Context, deps *CommandsDependencies, msg string) error {
	session, err := deps.SessionManager.GetOrCreateSession(ctx, deps.Context.UserID(), deps.Context.ChannelID(), &query.UserContext{
		UserID:      deps.Context.UserID(),
		SlackTeamID: deps.Context.TeamID(),
		IsAdmin:     false,
	})
	if err != nil {
		return send(ctx, deps, deps.Context.UserID(), "", "Error retrieving session")
	}

	tools := session.AvailableTools
	if len(tools) == 0 {
		return send(ctx, deps, deps.Context.UserID(), "", "You don't have any tools available yet. Try adding an HTTP MCP tool!")
	}

	var toolList strings.Builder
	toolList.WriteString("🔧 **Your Available Tools:**\n\n")

	for i, tool := range tools {
		fmt.Fprintf(&toolList, "%d. `%s`\n", i+1, tool)
	}

	return send(ctx, deps, deps.Context.UserID(), "", toolList.String())
}

func HandleRemoveTool(ctx context.Context, deps *CommandsDependencies, msg string) error {
	toolName := strings.TrimSpace(msg)
	if toolName == "" {
		return send(ctx, deps, deps.Context.UserID(), "", "❌ Please specify which tool to remove. Example: remove my-tool")
	}

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

	if err := deps.ToolSet.RemoveTool(ctx, deps.Context.UserID(), toolName); err != nil {
		deps.SecurityLogger.LogError(deps.Context.UserID(), "remove_tool", fmt.Sprintf("failed to remove tool: %v", err))
		return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("❌ Error removing tool: %s", err.Error()))
	}

	deps.SecurityLogger.LogToolRemoved(deps.Context.UserID(), toolName)
	return send(ctx, deps, deps.Context.UserID(), "", fmt.Sprintf("✅ Tool %q has been removed from your workspace", toolName))
}

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

func HandleModelAccess(ctx context.Context, deps *CommandsDependencies, msg string) error {
	return nil
}

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

func extractNameFromConfig(config map[string]interface{}) string {
	if name, ok := config["name"].(string); ok {
		return name
	}
	return "unnamed"
}

func send(ctx context.Context, deps *CommandsDependencies, userID, channelID, message string) error {
	if deps.MessageSender != nil {
		return deps.MessageSender.SendMessage(ctx, userID, message)
	}
	if userID != "" {
		return SendMessage(deps.SlackClient, userID, message)
	}
	return SendMessage(deps.SlackClient, channelID, message)
}
