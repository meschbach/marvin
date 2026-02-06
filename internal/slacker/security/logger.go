package security

import (
	"fmt"
	"io"
	"os"
	"time"
)

// SecurityLogger handles security-related logging to stdout
type SecurityLogger struct {
	output io.Writer
}

// NewSecurityLogger creates a new security logger
func NewSecurityLogger() *SecurityLogger {
	return &SecurityLogger{
		output: os.Stdout,
	}
}

// LogError logs security-related errors
func (sl *SecurityLogger) LogError(userID, component, error string) {
	fmt.Fprintf(sl.output, "[SECURITY] %s - Error - User: %s, Component: %s, Error: %s\n",
		time.Now().Format(time.RFC3339), userID, component, error)
}

// LogInfo logs informational events
func (sl *SecurityLogger) LogInfo(userID, component, message string) {
	fmt.Fprintf(sl.output, "[INFO] %s - User: %s, Component: %s, Message: %s\n",
		time.Now().Format(time.RFC3339), userID, component, message)
}

// LogDebug logs debug-level messages for development and troubleshooting
func (sl *SecurityLogger) LogDebug(userID, component, message string) {
	fmt.Fprintf(sl.output, "[DEBUG] %s - User: %s, Component: %s, Message: %s\n",
		time.Now().Format(time.RFC3339), userID, component, message)
}

// LogSessionEvent logs session-related events
func (sl *SecurityLogger) LogSessionEvent(userID, channelID, event string) {
	fmt.Fprintf(sl.output, "[DIAGNOSTIC] %s - Session Event - User: %s, Channel: %s, Event: %s\n",
		time.Now().Format(time.RFC3339), userID, channelID, event)
}

// LogBotConnection logs successful Slack bot connection
func (sl *SecurityLogger) LogBotConnection(botID, userID, username, teamID, teamName string) {
	fmt.Fprintf(sl.output, "[DIAGNOSTIC] %s - Bot Connected - BotID: %s, User: %s, Team: %s/%s\n",
		time.Now().Format(time.RFC3339), botID, userID, teamID, teamName)
}

// LogStartupEvent logs application startup events
func (sl *SecurityLogger) LogStartupEvent(event, details string) {
	fmt.Fprintf(sl.output, "[DIAGNOSTIC] %s - %s - %s\n",
		time.Now().Format(time.RFC3339), event, details)
}

// LogConfigChange logs when configuration is changed
func (sl *SecurityLogger) LogConfigChange(userID, configType, details string) {
	fmt.Fprintf(sl.output, "[DIAGNOSTIC] %s - Config Change - User: %s, Type: %s, Details: %s\n",
		time.Now().Format(time.RFC3339), userID, configType, details)
}

// LogToolRequest logs when a user requests to add a tool
func (sl *SecurityLogger) LogToolRequest(userID, toolType, config string) {
	fmt.Fprintf(sl.output, "[SECURITY] %s - Tool Request - User: %s, Type: %s, Config: %s\n",
		time.Now().Format(time.RFC3339), userID, toolType, config)
}

// LogToolAdded logs when a tool is successfully added for a user
func (sl *SecurityLogger) LogToolAdded(userID, toolID, toolType string) {
	fmt.Fprintf(sl.output, "[SECURITY] %s - Tool Added - User: %s, ToolID: %s, Type: %s\n",
		time.Now().Format(time.RFC3339), userID, toolID, toolType)
}

// LogToolApprovalRequired logs when a tool requires admin approval
func (sl *SecurityLogger) LogToolApprovalRequired(toolID, requesterID, toolType string, adminUsers []string) {
	fmt.Fprintf(sl.output, "[SECURITY] %s - Approval Required - ToolID: %s, Requester: %s, Type: %s, Admins: %v\n",
		time.Now().Format(time.RFC3339), toolID, requesterID, toolType, adminUsers)
}

// LogToolApproval logs when an admin approves or rejects a tool
func (sl *SecurityLogger) LogToolApproval(adminID, toolID, decision, reason string) {
	fmt.Fprintf(sl.output, "[SECURITY] %s - Tool Approval - Admin: %s, ToolID: %s, Decision: %s, Reason: %s\n",
		time.Now().Format(time.RFC3339), adminID, toolID, decision, reason)
}

// LogToolShare logs when a user shares a tool with another user
func (sl *SecurityLogger) LogToolShare(ownerID, targetID, toolID string) {
	fmt.Fprintf(sl.output, "[SECURITY] %s - Tool Share - Owner: %s, Target: %s, Tool: %s\n",
		time.Now().Format(time.RFC3339), ownerID, targetID, toolID)
}

// LogToolAccess logs when a user invokes a tool
func (sl *SecurityLogger) LogToolAccess(userID, toolID, operation string) {
	fmt.Fprintf(sl.output, "[SECURITY] %s - Tool Access - User: %s, ToolID: %s, Operation: %s\n",
		time.Now().Format(time.RFC3339), userID, toolID, operation)
}

// LogToolAccessDenied logs when tool access is denied
func (sl *SecurityLogger) LogToolAccessDenied(userID, toolID, reason string) {
	fmt.Fprintf(sl.output, "[SECURITY] %s - Access Denied - User: %s, ToolID: %s, Reason: %s\n",
		time.Now().Format(time.RFC3339), userID, toolID, reason)
}

// LogCredentialAccess logs when user credentials are accessed
func (sl *SecurityLogger) LogCredentialAccess(userID, operation string) {
	fmt.Fprintf(sl.output, "[SECURITY] %s - Credential Access - User: %s, Operation: %s\n",
		time.Now().Format(time.RFC3339), userID, operation)
}

// LogAdminAction logs any administrative action
func (sl *SecurityLogger) LogAdminAction(adminID, action, target string) {
	fmt.Fprintf(sl.output, "[SECURITY] %s - Admin Action - Admin: %s, Action: %s, Target: %s\n",
		time.Now().Format(time.RFC3339), adminID, action, target)
}

// LogConnectionState logs changes in Slack connection state
func (sl *SecurityLogger) LogConnectionState(fromState, toState, details string) {
	fmt.Fprintf(sl.output, "[CONNECTION] %s - State Change: %s -> %s - %s\n",
		time.Now().Format(time.RFC3339), fromState, toState, details)
}

// LogConnectionAttempt logs connection attempts with diagnostics
func (sl *SecurityLogger) LogConnectionAttempt(attempt int, maxAttempts int, reason string) {
	fmt.Fprintf(sl.output, "[CONNECTION] %s - Attempt %d/%d - %s\n",
		time.Now().Format(time.RFC3339), attempt, maxAttempts, reason)
}

// LogConnectionError logs detailed connection errors with context
func (sl *SecurityLogger) LogConnectionError(errorType, errorDetails, recoveryAction string) {
	fmt.Fprintf(sl.output, "[CONNECTION] %s - Error - Type: %s, Details: %s, Recovery: %s\n",
		time.Now().Format(time.RFC3339), errorType, errorDetails, recoveryAction)
}

// LogConnectionMetrics logs connection performance metrics
func (sl *SecurityLogger) LogConnectionMetrics(uptime, messagesProcessed, errorCount int, latency string) {
	fmt.Fprintf(sl.output, "[CONNECTION] %s - Uptime: %ds, Messages: %d, Errors: %d, Latency: %s\n",
		time.Now().Format(time.RFC3339), uptime, messagesProcessed, errorCount, latency)
}
