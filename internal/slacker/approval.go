package slacker

import (
	"fmt"
	"sync"
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

// ApprovalWorkflow manages the tool approval process
type ApprovalWorkflow struct {
	approvals     map[string]*query.ToolApproval // requestID -> approval
	pendingByUser map[string][]string            // userID -> []requestID
	adminUsers    map[string]bool
	logger        *sec.SecurityLogger
	mutex         sync.RWMutex

	// Slack integration
	notifyFunc func(*ToolApprovalRequest) error
}

// NewApprovalWorkflow creates a new approval workflow system
func NewApprovalWorkflow(adminUsers []string, logger *sec.SecurityLogger) *ApprovalWorkflow {
	aw := &ApprovalWorkflow{
		approvals:     make(map[string]*query.ToolApproval),
		pendingByUser: make(map[string][]string),
		adminUsers:    make(map[string]bool),
		logger:        logger,
	}

	for _, adminID := range adminUsers {
		aw.adminUsers[adminID] = true
	}

	return aw
}

// SetNotifyFunction sets the Slack notification function
func (aw *ApprovalWorkflow) SetNotifyFunction(notifyFunc func(*ToolApprovalRequest) error) {
	aw.notifyFunc = notifyFunc
}

// RequestToolApproval submits a tool for approval
func (aw *ApprovalWorkflow) RequestToolApproval(request *ToolApprovalRequest) (string, error) {
	aw.mutex.Lock()
	defer aw.mutex.Unlock()

	// Check if approval is needed
	if !config.RequiresApproval(request.ToolType) {
		// HTTP tools don't need approval
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
	aw.approvals[requestID] = approval

	// Add to user's pending list
	aw.pendingByUser[request.RequesterID] = append(aw.pendingByUser[request.RequesterID], requestID)

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
	aw.mutex.Lock()
	defer aw.mutex.Unlock()

	// Check if admin
	if !aw.adminUsers[adminID] {
		return fmt.Errorf("user %s is not an admin", adminID)
	}

	approval, exists := aw.approvals[requestID]
	if !exists {
		return fmt.Errorf("approval request %s not found", requestID)
	}

	// Update approval
	approval.Status = query.ApprovalStatusApproved
	approval.ApprovedBy = adminID
	approval.Reason = reason

	// Log approval
	aw.logger.LogToolApproval(adminID, requestID, "approved", reason)

	return nil
}

// RejectTool rejects a tool request
func (aw *ApprovalWorkflow) RejectTool(adminID, requestID, reason string) error {
	aw.mutex.Lock()
	defer aw.mutex.Unlock()

	// Check if admin
	if !aw.adminUsers[adminID] {
		return fmt.Errorf("user %s is not an admin", adminID)
	}

	approval, exists := aw.approvals[requestID]
	if !exists {
		return fmt.Errorf("approval request %s not found", requestID)
	}

	// Update approval
	approval.Status = query.ApprovalStatusRejected
	approval.ApprovedBy = adminID
	approval.Reason = reason

	// Log rejection
	aw.logger.LogToolApproval(adminID, requestID, "rejected", reason)

	return nil
}

// GetApprovalStatus gets the status of an approval request
func (aw *ApprovalWorkflow) GetApprovalStatus(requestID string) (*query.ToolApproval, error) {
	aw.mutex.RLock()
	defer aw.mutex.RUnlock()

	approval, exists := aw.approvals[requestID]
	if !exists {
		return nil, fmt.Errorf("approval request %s not found", requestID)
	}

	return approval, nil
}

// GetPendingApprovals gets all pending approvals for admins
func (aw *ApprovalWorkflow) GetPendingApprovals() map[string]*query.ToolApproval {
	aw.mutex.RLock()
	defer aw.mutex.RUnlock()

	pending := make(map[string]*query.ToolApproval)
	for requestID, approval := range aw.approvals {
		if approval.Status == query.ApprovalStatusPending {
			pending[requestID] = approval
		}
	}

	return pending
}

// GetUserApprovals gets all approvals for a specific user
func (aw *ApprovalWorkflow) GetUserApprovals(userID string) []*query.ToolApproval {
	aw.mutex.RLock()
	defer aw.mutex.RUnlock()

	var userApprovals []*query.ToolApproval
	requestIDs, exists := aw.pendingByUser[userID]
	if !exists {
		return userApprovals
	}

	for _, requestID := range requestIDs {
		if approval, exists := aw.approvals[requestID]; exists {
			userApprovals = append(userApprovals, approval)
		}
	}

	return userApprovals
}

// IsAdmin checks if a user is an admin
func (aw *ApprovalWorkflow) IsAdmin(userID string) bool {
	aw.mutex.RLock()
	defer aw.mutex.RUnlock()
	return aw.adminUsers[userID]
}

// getAdminUserList returns the list of admin user IDs
func (aw *ApprovalWorkflow) getAdminUserList() []string {
	aw.mutex.RLock()
	defer aw.mutex.RUnlock()

	admins := make([]string, 0, len(aw.adminUsers))
	for adminID := range aw.adminUsers {
		admins = append(admins, adminID)
	}

	return admins
}

// FormatApprovalForSlack formats an approval request for Slack notification
func (aw *ApprovalWorkflow) FormatApprovalForSlack(request *ToolApprovalRequest) string {
	requestID := request.RequesterID[:8] + "-" + fmt.Sprintf("%d", time.Now().Unix())

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
		aw.formatConfigForSlack(request.Config),
		requestID,
		requestID,
	)

	return message
}

// formatConfigForSlack formats tool configuration for Slack display
func (aw *ApprovalWorkflow) formatConfigForSlack(config interface{}) string {
	return fmt.Sprintf("%+v", config)
}

// CleanupOldApprovals removes old approvals (optional implementation)
func (aw *ApprovalWorkflow) CleanupOldApprovals(olderThan time.Duration) {
	aw.mutex.Lock()
	defer aw.mutex.Unlock()

	cutoff := time.Now().Add(-olderThan)
	var toDelete []string

	for requestID, approval := range aw.approvals {
		if approval.Timestamp.Before(cutoff) &&
			(approval.Status == query.ApprovalStatusRejected ||
				approval.Status == query.ApprovalStatusApproved) {
			toDelete = append(toDelete, requestID)
		}
	}

	// Delete old approvals
	for _, requestID := range toDelete {
		delete(aw.approvals, requestID)

		// Remove from user pending lists
		for userID, requestIDs := range aw.pendingByUser {
			var newRequestIDs []string
			for _, rid := range requestIDs {
				if rid != requestID {
					newRequestIDs = append(newRequestIDs, rid)
				}
			}
			aw.pendingByUser[userID] = newRequestIDs
		}
	}
}
