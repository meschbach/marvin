package slacker

import (
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
	store     *ApprovalStore
	admins    map[string]bool
	logger    *sec.SecurityLogger
	formatter *ApprovalFormatter

	// Slack integration
	notifyFunc func(*ToolApprovalRequest) error
}

// NewApprovalWorkflow creates a new approval workflow system
func NewApprovalWorkflow(adminUsers []string, logger *sec.SecurityLogger) *ApprovalWorkflow {
	aw := &ApprovalWorkflow{
		store:     NewApprovalStore(),
		admins:    make(map[string]bool),
		logger:    logger,
		formatter: NewApprovalFormatter(),
	}

	for _, adminID := range adminUsers {
		aw.admins[adminID] = true
	}

	return aw
}

// SetNotifyFunction sets the Slack notification function
func (aw *ApprovalWorkflow) SetNotifyFunction(notifyFunc func(*ToolApprovalRequest) error) {
	aw.notifyFunc = notifyFunc
}

// RequestToolApproval submits a tool for approval
func (aw *ApprovalWorkflow) RequestToolApproval(request *ToolApprovalRequest) (string, error) {
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
		if err := aw.notifyFunc(request); err != nil {
			return requestID, fmt.Errorf("notifying admins: %w", err)
		}
	}

	return requestID, nil
}

// ApproveTool approves a tool request
func (aw *ApprovalWorkflow) ApproveTool(adminID, requestID, reason string) error {
	// Check if admin
	if !aw.admins[adminID] {
		return fmt.Errorf("user %s is not an admin", adminID)
	}

	// Update approval
	if !aw.store.UpdateApproval(requestID, query.ApprovalStatusApproved, adminID, reason) {
		return fmt.Errorf("approval request %s not found", requestID)
	}

	// Log approval
	aw.logger.LogToolApproval(adminID, requestID, "approved", reason)

	return nil
}

// RejectTool rejects a tool request
func (aw *ApprovalWorkflow) RejectTool(adminID, requestID, reason string) error {
	// Check if admin
	if !aw.admins[adminID] {
		return fmt.Errorf("user %s is not an admin", adminID)
	}

	// Update approval
	if !aw.store.UpdateApproval(requestID, query.ApprovalStatusRejected, adminID, reason) {
		return fmt.Errorf("approval request %s not found", requestID)
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

// FormatApprovalForSlack formats an approval request for Slack notification
func (aw *ApprovalWorkflow) FormatApprovalForSlack(request *ToolApprovalRequest) string {
	return aw.formatter.FormatApprovalForSlack(request)
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
