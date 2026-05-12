# Tool Approval System

This document describes the tool approval workflow in Marvin's Slack integration, which provides secure tool management
with admin oversight for multi-tenant safety.

**🆕 Intelligent Help**: When tool commands fail or access is denied, Marvin's [intelligent help
system](intelligent-help.md) provides contextual assistance and guidance.

## Overview

The tool approval system ensures that potentially dangerous tools (local programs and Docker containers) require admin
approval before users can use them in their conversations. This provides security boundaries while maintaining
flexibility for legitimate use cases.

### Security Benefits

- **Multi-tenant isolation**: Users can only access tools they've been explicitly approved for
- **Admin oversight**: All potentially dangerous tool requests are reviewed by authorized admins
- **Audit trail**: Every request, approval, and rejection is logged for security analysis
- **Session-based access**: Approved tools are only available in the requesting user's sessions

### Tool Types and Approval Requirements

| Tool Type       | Approval Required | Examples                            | Risk Level |
|-----------------|-------------------|-------------------------------------|------------|
| `local_program` | **Yes**           | Executables, scripts on host system | High       |
| `docker_mcp`    | **Yes**           | Containerized MCP servers           | Medium     |
| `mcp_over_http` | **No**            | HTTP-based MCP servers              | Low        |

## Complete User Journey

### Phase 1: Tool Request

Users can request tools using natural language commands:

```
"Add local tool at /usr/bin/ls"
"Add docker tool nginx"
"Add http tool at https://api.example.com/mcp"
```

#### What Happens

1. **Intent Parsing**: The system parses the tool type and configuration
2. **Validation**: Tool configuration is validated for correctness and safety
3. **Request Submission**: If approval is required, the request is stored in the approval system
4. **Admin Notification**: All admins receive a DM with the request details
5. **User Confirmation**: The requester gets immediate confirmation:
   ```
   📋 Tool approval request submitted:
   • Tool ID: user-123-local-20260210-150405
   • Status: Pending approval
   • I'll notify you when it's approved.
   ```

### Phase 2: Admin Review

Admins receive detailed notification DMs for each approval request:

```
🔧 Tool Approval Request

• Requester: @john.doe
• Tool Type: local_program
• Tool ID: user-123-local-20260210-150405
• Timestamp: 2026-02-10 15:04:05
• Configuration:
  ```
  {
    "Name": "ls",
    "Command": "/usr/bin/ls"
  }
  ```

👉 Please review and approve/reject this request.
Reply with "Approve user-123-local-20260210-150405" or "Reject user-123-local-20260210-150405 because [reason]"
```

#### Admin Approval Methods

Admins can approve or reject using natural language:

```
"Approve user-123-local-20260210-150405"
"Reject user-123-local-20260210-150405 because security policy violation"
```

### Phase 3: Approval Processing

When an admin approves a request, the following happens immediately:

1. **Requester Notification**: The original user receives a DM:
   ```
   ✅ Your tool request has been approved!

   • Request ID: user-123-local-20260210-150405
   • Tool ID: user-123-local-20260210-150405
   • Approved by: @admin.user
   • Reason: Tool looks safe and legitimate

   The tool is now available in your conversations. Try using it!
   ```

2. **Tool Activation**: The approved tool is added to the user's session:
   - The tool becomes available in subsequent conversations
   - The user can immediately start using the tool
   - Tool availability persists across conversations

3. **Security Logging**: The approval is logged with full context:
   ```
   [INFO] Tool approval: user-123-local-20260210-150405 approved by @admin.user
   ```

#### Rejection Process

For rejected requests, the user receives:

```
❌ Your tool request has been rejected.

• Request ID: user-123-local-20260210-150405
• Tool ID: user-123-local-20260210-150405
• Rejected by: @admin.user
• Reason: Security policy violation - unrestricted file system access

If you believe this is an error, please contact an admin for review.
```

## Technical Architecture

### Component Diagram

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   User Request  │    │   ToolManager   │    │ ApprovalWorkflow│    │NotificationSender│
│                 │    │                 │    │                 │    │                 │
│ "Add local      │───▶│ParseToolConfig  │───▶│RequestToolApproval│───▶│NotifyAdmins     │
│ tool at /bin/ls"│    │ValidateConfig   │    │StoreApproval     │    │Send Admin DMs   │
└─────────────────┘    └─────────────────┘    └─────────────────┘    └─────────────────┘
                                                       │                        │
                                                       ▼                        ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ SessionManager  │◀───│ ApprovalWorkflow│◀───│ Admin Approval  │◀───│ Admin User      │
│                 │    │                 │    │                 │    │                 │
│ UpdateTools     │    │ApproveTool      │    │Natural Language │    │"Approve ID"     │
│ ActivateTool    │    │RejectTool       │    │Command          │    │"Reject ID"      │
└─────────────────┘    └─────────────────┘    └─────────────────┘    └─────────────────┘
```

### Data Flow

```
1. User: "Add local tool at /usr/bin/ls"
   ↓
2. ToolManager.handleAddTool()
   - Parse tool configuration
   - Validate tool type requires approval
   ↓
3. ApprovalWorkflow.RequestToolApproval()
   - Generate unique request ID
   - Store approval request
   - Log security event
   ↓
4. NotificationSender.NotifyAdmins()
   - Format approval request message
   - Send DMs to all admin users
   ↓
5. Admin: "Approve user-123-local-20260210-150405"
   ↓
6. ApprovalWorkflow.ApproveTool()
   - Verify admin permissions
   - Update approval status
   - Log approval decision
   ↓
7. NotificationSender.SendApprovalNotification()
   - Send approval notification to original requester
   ↓
8. SessionManager.UpdateAvailableTools()
   - Add approved tool to user's session
   - Persist tool availability
```

### Component Responsibilities

#### ToolManager
- **Intent Parsing**: Converts natural language to structured tool requests
- **Configuration Validation**: Validates tool configuration syntax and safety
- **Error Handling**: Provides helpful error messages for invalid configurations
- **User Communication**: Sends immediate confirmations to requesters

#### ApprovalWorkflow
- **Business Logic**: Enforces approval requirements and admin permissions
- **State Management**: Tracks approval status and request lifecycle
- **Security Logging**: Logs all approval actions with full context
- **Policy Enforcement**: Validates admin permissions and request validity

#### SessionManager
- **Tool Activation**: Manages approved tools in user sessions
- **Persistence**: Maintains tool availability across conversations
- **Isolation**: Ensures users only access their approved tools
- **Session Cleanup**: Manages session lifecycle and cleanup

#### NotificationSender
- **Admin Notifications**: Delivers detailed approval requests to admins
- **Requester Notifications**: Sends approval/rejection status to users
- **Slack Integration**: Handles DM creation and message formatting
- **Error Handling**: Logs notification failures (no retry per policy)

## Implementation Status

### ✅ Working Features

- **Tool Request Parsing**: Natural language intent recognition for tool requests
- **Configuration Validation**: Syntax validation for all tool types
- **Admin Notifications**: Immediate DM notification to all admin users
- **Approval Commands**: Natural language approval/rejection commands for admins
- **Security Logging**: Complete audit trail of all approval actions
- **Session Management**: Tool isolation and availability per user
- **Permission Enforcement**: Admin-only approval and rejection capabilities

### ✅ Implementation Gaps Resolved

#### ✅ Fixed: Broken User Notification Promise
**Status**: Users now receive notifications when their tool requests are approved or rejected.

**Implementation**:
- Added `SendApprovalNotification()` calls in `ApproveTool()` and `RejectTool()` methods
- Implemented rich notifications to original requester with status, tool details, and approving admin
- Enhanced notification formats per documentation:
  - Approval notifications with ✅ emoji and activation message
  - Rejection notifications with ❌ emoji and contact information
- Fulfilled the user experience promise made in initial confirmation

#### ✅ Fixed: Missing Tool Activation
**Status**: Approved tools are automatically added to user sessions upon approval.

**Implementation**:
- Integrated `SessionManager` with `ApprovalWorkflow`
- Added `activateToolForUser()` helper method
- Tools are now available in user conversations immediately after approval
- Session persistence maintained across conversations

#### ✅ Resolved: Duplicate Approval Formatter Code
**Status**: Consolidated to single, working approval formatter.

**Implementation**:
- Removed unused `ApprovalFormatter.FormatApprovalForSlack()` and `approval_formatter.go` file
- Enhanced working admin notifications with explicit command examples
- Added request IDs and clear approval/rejection instructions
- Consolidated all approval formatting logic in `NotificationSender`

#### ✅ Enhanced: Admin Instructions
**Status**: Admin notifications now include explicit approval commands.

**Implementation**:
- Added "Reply with 'Approve REQUEST_ID'" and "Reply with 'Reject REQUEST_ID because [reason]'" instructions
- Included request IDs for easy reference and command targeting
- Improved formatting for better admin usability

## Security & Audit

### Logging Events

All approval actions are logged with full context:

```
[INFO] Tool request: user-123-local-20260210-150405 requested by @john.doe
[INFO] Tool approval: user-123-local-20260210-150405 approved by @admin.user
[INFO] Tool rejection: user-123-local-20260210-150405 rejected by @admin.user
```

### Security Boundaries

- **Admin Isolation**: Only designated admins can approve/reject requests
- **User Isolation**: Users can only access their own approved tools
- **Session Isolation**: Tool availability is scoped to specific user sessions
- **Configuration Validation**: Prevents injection attacks via tool configurations

### Data Privacy

- **Minimal Data Collection**: Only necessary tool configuration is stored
- **User Consent**: Users explicitly request tools and approve notifications
- **Access Control**: Approval requests and configurations are admin-only visible

## Error Handling

### Notification Failures
- **Policy**: Log failures only, no retry mechanism
- **Rationale**: Approval status is stored permanently and can be checked manually
- **Logging**: All notification failures are logged for admin visibility

### Configuration Errors
- **Immediate Feedback**: Users receive specific error messages for invalid configurations
- **Helpful Messages**: Errors include suggestions for fixing configuration issues
- **Security**: Error messages don't expose sensitive system information

### Permission Errors
- **Admin Enforcement**: Non-admin approval attempts are rejected and logged
- **User Feedback**: Clear messaging about permission requirements
- **Security Logging**: Unauthorized attempts are logged for security review

## Future Enhancements

### ✅ Short-term Improvements Completed
1. **✅ Fixed Broken Notifications**: Implemented requester notifications for approval/rejection
2. **✅ Removed Duplicate Code**: Consolidated approval formatters
3. **✅ Enhanced Admin Instructions**: Included explicit command examples

### Medium-term Features
1. **Tool Sharing**: Allow users to share approved tools with other users
2. **Bulk Approvals**: Enable admins to approve multiple requests at once
3. **Tool Categories**: Organize tools by type and purpose
4. **Expiration Policies**: Time-based expiration of approved tools

### Long-term Vision
1. **Automated Security Analysis**: AI-powered risk assessment for tool requests
2. **Integration with Corporate Systems**: Connect with existing IT approval workflows
3. **Usage Analytics**: Track tool usage patterns and optimize approval policies
4. **Multi-admin Workflows**: Escalation and approval chains for different tool types

## API Reference

### Tool Request Commands

| Command                                    | Tool Type     | Example                                    | Result            |
|--------------------------------------------|---------------|--------------------------------------------|-------------------|
| "Add local tool at /path/to/executable"    | local_program | "Add local tool at /usr/bin/ls"            | Requires approval |
| "Add docker tool nginx"                    | docker_mcp    | "Add docker tool nginx"                    | Requires approval |
| "Add http tool at https://api.example.com" | mcp_over_http | "Add http tool at https://api.example.com" | Auto-approved     |

### Admin Commands

| Command                            | Action               | Example                                                         |
|------------------------------------|----------------------|-----------------------------------------------------------------|
| "Approve REQUEST_ID"               | Approve tool request | "Approve user-123-local-20260210-150405"                        |
| "Reject REQUEST_ID because REASON" | Reject tool request  | "Reject user-123-local-20260210-150405 because security policy" |

### Management Commands

| Command          | Action                        | Example                                      |
|------------------|-------------------------------|----------------------------------------------|
| "List tools"     | Show user's available tools   | "List tools"                                 |
| "Remove tool ID" | Remove tool from user session | "Remove tool user-123-local-20260210-150405" |

---

*This documentation reflects the current implementation state and the intended user experience. See the Implementation Status section for known gaps and improvement opportunities.*