package slacker

import (
	"sync"

	"github.com/meschbach/marvin/internal/query"
)

// ApprovalStore handles storage and basic operations for tool approvals
type ApprovalStore struct {
	approvals     map[string]*query.ToolApproval // requestID -> approval
	pendingByUser map[string][]string            // userID -> []requestID
	mutex         sync.RWMutex
}

// NewApprovalStore creates a new approval store
func NewApprovalStore() *ApprovalStore {
	return &ApprovalStore{
		approvals:     make(map[string]*query.ToolApproval),
		pendingByUser: make(map[string][]string),
	}
}

// StoreApproval stores a new approval request
func (as *ApprovalStore) StoreApproval(requestID string, approval *query.ToolApproval, requesterID string) {
	as.mutex.Lock()
	defer as.mutex.Unlock()

	as.approvals[requestID] = approval
	as.pendingByUser[requesterID] = append(as.pendingByUser[requesterID], requestID)
}

// GetApproval retrieves an approval by request ID
func (as *ApprovalStore) GetApproval(requestID string) (*query.ToolApproval, bool) {
	as.mutex.RLock()
	defer as.mutex.RUnlock()

	approval, exists := as.approvals[requestID]
	return approval, exists
}

// UpdateApproval updates an approval's status
func (as *ApprovalStore) UpdateApproval(requestID string, status query.ApprovalStatus, approvedBy, reason string) bool {
	as.mutex.Lock()
	defer as.mutex.Unlock()

	approval, exists := as.approvals[requestID]
	if !exists {
		return false
	}

	approval.Status = status
	approval.ApprovedBy = approvedBy
	approval.Reason = reason

	return true
}

// GetAllPendingApprovals returns all pending approvals
func (as *ApprovalStore) GetAllPendingApprovals() map[string]*query.ToolApproval {
	as.mutex.RLock()
	defer as.mutex.RUnlock()

	pending := make(map[string]*query.ToolApproval)
	for requestID, approval := range as.approvals {
		if approval.Status == query.ApprovalStatusPending {
			pending[requestID] = approval
		}
	}

	return pending
}

// GetUserApprovals returns all approvals for a specific user
func (as *ApprovalStore) GetUserApprovals(userID string) []*query.ToolApproval {
	as.mutex.RLock()
	defer as.mutex.RUnlock()

	var userApprovals []*query.ToolApproval
	requestIDs, exists := as.pendingByUser[userID]
	if !exists {
		return userApprovals
	}

	for _, requestID := range requestIDs {
		if approval, exists := as.approvals[requestID]; exists {
			userApprovals = append(userApprovals, approval)
		}
	}

	return userApprovals
}

// RemoveOldApprovals removes approvals older than the specified duration
func (as *ApprovalStore) RemoveOldApprovals(cutoffTime int64) []string {
	as.mutex.Lock()
	defer as.mutex.Unlock()

	var toDelete []string

	for requestID, approval := range as.approvals {
		if approval.Timestamp.Unix() < cutoffTime &&
			(approval.Status == query.ApprovalStatusRejected ||
				approval.Status == query.ApprovalStatusApproved) {
			toDelete = append(toDelete, requestID)
		}
	}

	// Delete old approvals
	for _, requestID := range toDelete {
		delete(as.approvals, requestID)

		// Remove from user pending lists
		for userID, requestIDs := range as.pendingByUser {
			var newRequestIDs []string
			for _, rid := range requestIDs {
				if rid != requestID {
					newRequestIDs = append(newRequestIDs, rid)
				}
			}
			as.pendingByUser[userID] = newRequestIDs
		}
	}

	return toDelete
}
