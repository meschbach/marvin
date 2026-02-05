package slacker

// OutgoingMessages defines the interface for notification senders
type OutgoingMessages interface {
	SendMessage(userID, message string) error
	NotifyAdmins(request *ToolApprovalRequest) error
	SendApprovalNotification(adminID, requestID, status string) error
}
