package slacker

import (
	"context"
	"fmt"

	"github.com/meschbach/marvin/internal/query"
	"github.com/meschbach/marvin/internal/slacker/commands"
	sec "github.com/meschbach/marvin/internal/slacker/security"
)

// ToolManagerImpl handles tool management operations
type ToolManagerImpl struct {
	approvalWorkflow   *ApprovalWorkflow
	tenantToolSet      *query.TenantToolSet
	securityLogger     *sec.SecurityLogger
	notificationSender OutgoingMessages
	sessionManager     *SessionManager
	helpIntegrator     *HelpIntegrator
}

// NewToolManager creates a new tool manager
func NewToolManager(
	approvalWorkflow *ApprovalWorkflow,
	tenantToolSet *query.TenantToolSet,
	securityLogger *sec.SecurityLogger,
	notificationSender OutgoingMessages,
	sessionManager *SessionManager,
	helpIntegrator *HelpIntegrator,
) *ToolManagerImpl {
	return &ToolManagerImpl{
		approvalWorkflow:   approvalWorkflow,
		tenantToolSet:      tenantToolSet,
		securityLogger:     securityLogger,
		notificationSender: notificationSender,
		sessionManager:     sessionManager,
		helpIntegrator:     helpIntegrator,
	}
}

// GetApprovalWorkflow returns the approval workflow
func (tm *ToolManagerImpl) GetApprovalWorkflow() *ApprovalWorkflow {
	return tm.approvalWorkflow
}

// HandleToolIntent processes tool management intents
func (tm *ToolManagerImpl) HandleToolIntent(ctx context.Context, slackCtx *SlackContext, session *UserSession, intent *ToolManagementIntent) error {
	deps := &commands.CommandsDependencies{
		Context:          &intentContextAdapter{slackCtx: slackCtx},
		ApprovalWorkflow: &approvalWorkflowAdapterForToolManager{tm.approvalWorkflow},
		TenantToolSet:    tm.tenantToolSet,
		SecurityLogger:   tm.securityLogger,
		SessionManager:   &sessionManagerAdapterForToolManager{tm.sessionManager},
		SlackClient:      tm.notificationSender.GetClient(),
		ToolParser:       &toolParserAdapter{},
		MessageSender:    tm.notificationSender,
	}

	switch intent.Action {
	case "add_tool":
		configStr, _ := intent.Config.(string)
		return commands.HandleAddTool(ctx, deps, "http "+intent.ToolType+" "+configStr)
	case "share_tool":
		return commands.HandleShareTool(ctx, deps, "")
	case "list_tools":
		return commands.HandleListTools(ctx, deps, "")
	case "remove_tool":
		return commands.HandleRemoveTool(ctx, deps, "")
	case "approve_tool":
		return commands.HandleApprove(ctx, deps, "approve "+intent.Target)
	case "reject_tool":
		return commands.HandleReject(ctx, deps, "reject "+intent.Target)
	case "reset_session":
		return commands.HandleResetSession(ctx, deps, "")
	default:
		return tm.notificationSender.SendMessage(ctx, slackCtx.UserID, fmt.Sprintf("I don't know how to handle: %s", intent.Action))
	}
}

type intentContextAdapter struct {
	slackCtx *SlackContext
}

func (a *intentContextAdapter) UserID() string    { return a.slackCtx.UserID }
func (a *intentContextAdapter) UserName() string  { return a.slackCtx.UserName }
func (a *intentContextAdapter) ChannelID() string { return a.slackCtx.ChannelID }
func (a *intentContextAdapter) TeamID() string    { return a.slackCtx.TeamID }

type sessionManagerAdapterForToolManager struct {
	sm *SessionManager
}

func (s *sessionManagerAdapterForToolManager) ClearSession(userID, channelID string) error {
	return s.sm.ClearSession(userID, channelID)
}

func (s *sessionManagerAdapterForToolManager) GetOrCreateSession(ctx context.Context, userID, channelID string, userCtx *query.UserContext) (*commands.UserSession, error) {
	session := s.sm.GetOrCreateSession(ctx, userID, channelID, userCtx)
	return &commands.UserSession{
		UserID:         session.UserID,
		Context:        session.UserContext,
		AvailableTools: session.AvailableTools,
	}, nil
}

type approvalWorkflowAdapterForToolManager struct {
	aw *ApprovalWorkflow
}

func (a *approvalWorkflowAdapterForToolManager) RequestToolApproval(ctx context.Context, request *commands.ToolApprovalRequest) (string, error) {
	toolReq := &ToolApprovalRequest{
		ToolID:        request.ToolID,
		RequesterID:   request.RequesterID,
		ToolType:      request.ToolType,
		Config:        request.Config,
		RequesterName: request.RequesterName,
	}
	return a.aw.RequestToolApproval(ctx, toolReq)
}

func (a *approvalWorkflowAdapterForToolManager) ApproveTool(ctx context.Context, approverID, requestID, reason string) error {
	return a.aw.ApproveTool(ctx, approverID, requestID, reason)
}

func (a *approvalWorkflowAdapterForToolManager) RejectTool(ctx context.Context, approverID, requestID, reason string) error {
	return a.aw.RejectTool(ctx, approverID, requestID, reason)
}

func (a *approvalWorkflowAdapterForToolManager) IsAdmin(userID string) bool {
	return a.aw.IsAdmin(userID)
}

// provideToolConfigHelp provides intelligent help when tool configuration fails
func (tm *ToolManagerImpl) provideToolConfigHelp(ctx context.Context, slackCtx *SlackContext, toolType, configStr string, err error) {
	if tm.helpIntegrator == nil {
		return
	}

	// Analyze the tool configuration error
	analysis, err := tm.helpIntegrator.HandleToolConfigError(ctx, slackCtx.UserID, slackCtx.ChannelID, toolType, configStr, err)
	if err != nil {
		tm.securityLogger.LogError(slackCtx.UserID, "help_system",
			fmt.Sprintf("Failed to provide tool config help: %v", err))
		return
	}

	// Only show help if confidence is above threshold
	if !ShouldShowHelp(analysis) {
		return
	}

	// Create help response
	helpResponse := tm.helpIntegrator.CreateHelpResponse(analysis)

	// Log the help for now - in a full implementation we'd send it via the notification system
	tm.securityLogger.LogInfo(slackCtx.UserID, "help_system",
		fmt.Sprintf("Tool config help prepared (confidence: %.2f): %s", analysis.Confidence, helpResponse.QuickText))
}
