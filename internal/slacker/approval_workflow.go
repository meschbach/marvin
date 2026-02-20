package slacker

import (
	"context"
	"fmt"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/query"
	sec "github.com/meschbach/marvin/internal/slacker/security"
)

// ToolApprovalRequest represents a request for tool approval
type ToolApprovalRequest struct {
	ToolID        string
	RequesterID   string
	ToolType      string      // "local_program", "docker_mcp", "mcp_over_http"
	Config        interface{} // the actual tool config
	RequesterName string      // Slack user display name
	Reason        string      // optional reason for request
	Timestamp     time.Time
}

// ApprovalWorkflow manages the tool approval process with business logic
type ApprovalWorkflow struct {
	store              *ApprovalStore
	admins             map[string]bool
	logger             *sec.SecurityLogger
	notificationSender *NotificationSender
	sessionManager     *SessionManager

	// Slack integration
	notifyFunc func(ctx context.Context, request *ToolApprovalRequest) error
}

// NewApprovalWorkflow creates a new approval workflow system
func NewApprovalWorkflow(adminUsers []string, logger *sec.SecurityLogger) *ApprovalWorkflow {
	aw := &ApprovalWorkflow{
		store:  NewApprovalStore(),
		admins: make(map[string]bool),
		logger: logger,
	}

	for _, adminID := range adminUsers {
		aw.admins[adminID] = true
	}

	return aw
}

// SetNotificationSender sets the notification sender for requester notifications
func (aw *ApprovalWorkflow) SetNotificationSender(notificationSender *NotificationSender) {
	aw.notificationSender = notificationSender
}

// SetSessionManager sets the session manager for tool activation
func (aw *ApprovalWorkflow) SetSessionManager(sessionManager *SessionManager) {
	aw.sessionManager = sessionManager
}

// SetNotifyFunction sets the Slack notification function
func (aw *ApprovalWorkflow) SetNotifyFunction(notifyFunc func(ctx context.Context, request *ToolApprovalRequest) error) {
	aw.notifyFunc = notifyFunc
}

// RequestToolApproval submits a tool for approval
func (aw *ApprovalWorkflow) RequestToolApproval(ctx context.Context, request *ToolApprovalRequest) (string, error) {
	// Check if approval is needed
	if !config.RequiresApproval(request.ToolType) {
		return "", fmt.Errorf("tool type %s does not require approval", request.ToolType)
	}

	// Generate request ID
	requestID := GenerateApprovalRequestID(request.ToolType, request.RequesterID)

	// Create approval record
	approval := &query.ToolApproval{
		ToolID:      request.ToolID,
		RequesterID: request.RequesterID,
		ToolType:    request.ToolType,
		Config:      request.Config,
		Status:      query.ApprovalStatusPending,
		Timestamp:   request.Timestamp,
	}

	// Store approval
	aw.store.StoreApproval(requestID, approval, request.RequesterID)

	// Log the request
	aw.logger.LogToolApprovalRequired(requestID, request.RequesterID, request.ToolType, aw.getAdminUserList())

	// Send notification to admins
	if aw.notifyFunc != nil {
		if err := aw.notifyFunc(ctx, request); err != nil {
			return requestID, fmt.Errorf("notifying admins: %w", err)
		}
	}

	return requestID, nil
}

// ApproveTool approves a tool request
func (aw *ApprovalWorkflow) ApproveTool(ctx context.Context, adminID, requestID, reason string) error {
	// Check if admin
	if !aw.admins[adminID] {
		return fmt.Errorf("user %s is not an admin", adminID)
	}

	// Update approval
	if !aw.store.UpdateApproval(requestID, query.ApprovalStatusApproved, adminID, reason) {
		return fmt.Errorf("approval request %s not found", requestID)
	}

	// Get approval details for notification
	approval, exists := aw.store.GetApproval(requestID)
	if !exists {
		return fmt.Errorf("approval request %s not found", requestID)
	}

	// Notify original requester
	if aw.notificationSender != nil {
		if err := aw.notificationSender.SendApprovalNotification(
			ctx,
			approval.RequesterID, // Send to requester, not admin
			adminID,              // Admin who approved
			requestID,            // Request ID
			"approved",           // Status
			approval.ToolID,      // Tool details
			reason,               // Reason for approval
		); err != nil {
			aw.logger.LogError("system", "NotificationSender", err.Error())
		}
	}

	// Activate tool in user's session
	if aw.sessionManager != nil {
		if err := aw.activateToolForUser(approval.RequesterID, approval.ToolID); err != nil {
			aw.logger.LogError("system", "SessionManager", err.Error())
		}
	}

	// Log approval
	aw.logger.LogToolApproval(adminID, requestID, "approved", reason)

	return nil
}

// RejectTool rejects a tool request
func (aw *ApprovalWorkflow) RejectTool(ctx context.Context, adminID, requestID, reason string) error {
	// Check if admin
	if !aw.admins[adminID] {
		return fmt.Errorf("user %s is not an admin", adminID)
	}

	// Update approval
	if !aw.store.UpdateApproval(requestID, query.ApprovalStatusRejected, adminID, reason) {
		return fmt.Errorf("approval request %s not found", requestID)
	}

	// Get approval details for notification
	approval, exists := aw.store.GetApproval(requestID)
	if !exists {
		return fmt.Errorf("approval request %s not found", requestID)
	}

	// Notify original requester
	if aw.notificationSender != nil {
		if err := aw.notificationSender.SendApprovalNotification(
			ctx,
			approval.RequesterID, // Send to requester, not admin
			adminID,              // Admin who rejected
			requestID,            // Request ID
			"rejected",           // Status
			approval.ToolID,      // Tool details
			reason,               // Reason for rejection
		); err != nil {
			aw.logger.LogError("system", "NotificationSender", err.Error())
		}
	}

	// Log rejection
	aw.logger.LogToolApproval(adminID, requestID, "rejected", reason)

	return nil
}

// GetApprovalStatus gets the status of an approval request
func (aw *ApprovalWorkflow) GetApprovalStatus(requestID string) (*query.ToolApproval, error) {
	approval, exists := aw.store.GetApproval(requestID)
	if !exists {
		return nil, fmt.Errorf("approval request %s not found", requestID)
	}

	return approval, nil
}

// GetPendingApprovals gets all pending approvals for admins
func (aw *ApprovalWorkflow) GetPendingApprovals() map[string]*query.ToolApproval {
	return aw.store.GetAllPendingApprovals()
}

// GetUserApprovals gets all approvals for a specific user
func (aw *ApprovalWorkflow) GetUserApprovals(userID string) []*query.ToolApproval {
	return aw.store.GetUserApprovals(userID)
}

// IsAdmin checks if a user is an admin
func (aw *ApprovalWorkflow) IsAdmin(userID string) bool {
	return aw.admins[userID]
}

// CleanupOldApprovals removes old approvals
func (aw *ApprovalWorkflow) CleanupOldApprovals(olderThan time.Duration) {
	cutoff := time.Now().Add(-olderThan)
	removedIDs := aw.store.RemoveOldApprovals(cutoff.Unix())

	for _, requestID := range removedIDs {
		aw.logger.LogSessionEvent("system", "approval_cleanup", fmt.Sprintf("Removed old approval: %s", requestID))
	}
}

// getAdminUserList returns the list of admin user IDs
func (aw *ApprovalWorkflow) getAdminUserList() []string {
	admins := make([]string, 0, len(aw.admins))
	for adminID := range aw.admins {
		admins = append(admins, adminID)
	}

	return admins
}

// activateToolForUser adds an approved tool to the user's session
func (aw *ApprovalWorkflow) activateToolForUser(userID, toolID string) error {
	if aw.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}

	// Find user's active sessions and add the approved tool
	sessions := aw.sessionManager.ListSessions()
	for _, session := range sessions {
		if session.UserID == userID {
			// Add tool to existing session
			currentTools := session.AvailableTools
			if !contains(currentTools, toolID) {
				newTools := append(currentTools, toolID)
				if err := aw.sessionManager.UpdateAvailableTools(userID, session.ChannelID, newTools); err != nil {
					return fmt.Errorf("updating tools for user %s: %w", userID, err)
				}
			}
		}
	}
	return nil
}

// contains checks if a string slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
