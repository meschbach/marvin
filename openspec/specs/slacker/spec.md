# Slacker Specification

Slacker is the multi-tenant Slack bot component of Marvin, providing enterprise Slack integration with admin approval workflows, per-user session isolation, and natural language tool management.

## Overview

Slacker connects to Slack via Socket Mode API, enabling real-time bidirectional communication with users in Slack channels and DMs. It provides an AI-powered conversational interface where users can interact with MCP tools through natural language.

### Entry Point
- **File**: `cmd/slacker/main.go`

### Environment Variables
| Variable | Format | Description |
|----------|--------|-------------|
| `SLACK_BOT_TOKEN` | `xoxb-...` | Bot user authentication token |
| `SLACK_APP_TOKEN` | `xapp-...` | App-level token for Socket Mode |

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Slacker Bot                          │
├─────────────────────────────────────────────────────────────┤
│  cmd/slacker/main.go                                     │
│         │                                                │
│         ▼                                              │
│  internal/slacker/bot.go (SlackBot)                     │
│         │                                                │
│    ┌────┴───────┬──────────────┬──────────┐          │
│    ▼            ▼              ▼          ▼          │
│ Events      Message       Query        Session        │
│ Router     Handler       Processor   Manager         │
│ (events.go) (message_     (query_     (session_        │
│            handler.go)    handler.go) manager.go)      │
│                                                          │
│    ┌──────────────┬─────────────────────┐            │
│    ▼              ▼                     ▼            │
│ Approval   Commands              Security              │
│ Workflow  (permissions,        (credentials,          │
│ (approval_ help,                logger,               │
│  workflow.go) modelaccess)      user_credentials)       │
│                                                          │
└─────────────────────────────────────────────────────────────┘
                  │
                  ▼
        Slack Socket Mode API
```

## Core Components

### 1. Message Handling

**Primary File**: `internal/slacker/message_handler.go`

The message handler processes incoming Slack messages and routes them to appropriate handlers. It supports both command-prefixed messages (in DMs) and mentions in channels.

| Element | Line | Description |
|--------|------|-------------|
| `MessageHandler` struct | `:92` | Main message processing orchestrator |
| `ProcessMessage` | `:340` | Entry point for message processing |
| `handleCommand` | `:493` | Processes command-prefixed messages |
| `routeMessage` | `:572` | Routes messages to command registry or query handler |

**Related Files**:
- `internal/slacker/events.go` - Socket Mode event routing (`EventRouter`)
- `internal/slacker/query_handler.go` - Query processing logic

**Supported Commands** (registered at `:124-138`):
| Command | Handler | Description |
|---------|---------|-------------|
| `help` | `commands.HandleHelp` | Show help message |
| `tools` | `commands.HandleTools` | List available tools |
| `list tools` | `commands.HandleListTools` | List all configured tools |
| `add tool` | `commands.HandleAddTool` | Add a new tool |
| `remove tool` | `commands.HandleRemoveTool` | Remove a tool |
| `reset session` | `commands.HandleResetSession` | Clear conversation history |
| `approve` | `commands.HandleApprove` | Approve a tool request |
| `reject` | `commands.HandleReject` | Reject a tool request |
| `share tool` | `commands.HandleShareTool` | Share tool with another user |
| `thinking` | `commands.HandleThinking` | Toggle thinking display |
| `done` | `commands.HandleDone` | Toggle done message display |
| `verbose` | `commands.HandleVerbose` | Toggle verbose output |
| `preferences` | `commands.HandlePreferences` | Manage user preferences |
| `admin` | `commands.HandleAdminHelp` | Show admin help |
| `model access` | `commands.HandleModelAccess` | Manage model access control |

### 2. Session Management

**Primary File**: `internal/slacker/session_manager.go`

Manages user sessions with persistence to disk. Each user-channel pair gets an isolated session.

| Element | Line | Description |
|--------|------|-------------|
| `SessionManager` struct | `:21` | In-memory session storage with persistence |
| `GetOrCreateSession` | `:47` | Gets or creates a user session |
| `ClearSession` | `:111` | Clears session messages |
| `ResolveUserPreferences` | `:209` | Resolves preferences (session > HCL > defaults) |
| `CleanupOldSessions` | `:251` | Removes expired sessions |

**Session File Format**:
```
sessions/session-{userID}-{channelID}.json
```

**User Session Structure** (`internal/slacker/user_session.go`):
| Field | Type | Description |
|-------|------|-------------|
| `UserID` | string | Slack user ID |
| `ChannelID` | string | Channel ID |
| `ThreadTS` | string | Thread timestamp |
| `LastActivity` | time.Time | Last activity timestamp |
| `Messages` | []api.Message | Conversation history |
| `AvailableTools` | []string | User's tools |
| `ToolNamespace` | string | Tool namespace |
| `Preferences` | UserPreferences | Display preferences |

### 3. Multi-Tenant Configuration

**Configuration Block**: `internal/config/multitenant.go:76-84`

```hcl
multi_tenant {
  admin_users        = ["U1234567890", "U0987654321"]
  admin_channel     = "C1234567890"
  session_store_path = "./sessions"
  credential_store  = "./credentials"
  slacker_state_path = "./slacker-state"
  security_log_format = "json"
  approval_timeout  = "24h"
  cron {
    spec = "0 9 * * *"
    message = "Daily standup reminder"
    target = ["U1234567890", "C0987654321"]
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `AdminUsers` | []string | Admin Slack user IDs |
| `AdminChannel` | string | Admin notification channel |
| `SessionStorePath` | string | Session persistence directory |
| `CredentialStore` | string | Encrypted credentials directory |
| `SlackerStatePath` | string | State files directory |
| `SecurityLogFormat` | string | Log format (json/plain) |
| `ApprovalTimeout` | string | Approval request timeout |
| `CronJobs` | []CronStanza | Scheduled jobs |

**File Reference**: `internal/config/file.go:110` (MultiTenant field)

### 4. Approval Workflows

**Primary File**: `internal/slacker/approval_workflow.go`

Security-sensitive tools (local programs, Docker MCP) require admin approval before activation.

| Element | Line | Description |
|--------|------|-------------|
| `ApprovalWorkflow` struct | `:25` | Approval orchestration |
| `RequestToolApproval` | `:67` | Submit tool for approval |
| `ApproveTool` | `:103` | Admin approves tool |
| `RejectTool` | `:149` | Admin rejects tool |
| `IsAdmin` | `:208` | Check admin status |

**Store**: `internal/slacker/approval_store.go`
| Element | Line | Description |
|--------|------|-------------|
| `ApprovalStore` struct | `:10` | In-memory approval storage |
| `StoreApproval` | `:25` | Store new request |
| `GetApproval` | `:34` | Get approval by ID |
| `UpdateApproval` | `:43` | Update status |
| `GetAllPendingApprovals` | `:60` | List pending approvals |

**Tools Requiring Approval** (`internal/config/multitenant.go:33-42`):
- `local_program`
- `docker_mcp`

**Approval Flow**:
```
User requests tool → ToolApprovalRequest → Admin Notification
                                          │
                                    ┌────────┼────────┐
                                    ▼        ▼        ▼
                                Approve   Reject   Timeout
                                    │        │        │
                                    ▼        ▼        ▼
                              Tool Activated  Notification  Request Expired
```

### 5. Tool Management

**Command Handlers**: `internal/slacker/commands/`

| File | Handler | Description |
|------|---------|-------------|
| `permissions.go` | `IsAdmin()`, `SendPermissionDenied()` | Permission checks |
| `help.go` | `RenderHelp()` | Help text rendering |
| `modelaccess.go` | `HandleModelAccess()` | Model access control |
| `preferences.go` | `HandlePreferences()` | User preferences |

**Permission Flow**:
```go
// internal/slacker/message_handler.go:515
if !mh.isAdminCommand(cmdName) || mh.tenantToolSet.IsAdmin(ev.User) {
    // Execute command
}
```

### 6. Cron Jobs

**Primary File**: `internal/slacker/cron_dispatcher.go`

| Element | Line | Description |
|--------|------|-------------|
| `CronDispatcher` struct | `:14` | Cron job execution |
| `OnTrigger` | `:28` | Execute scheduled job |

**Cron Configuration**: `internal/config/cron.go`

```hcl
cron {
  spec    = "0 9 * * *"      # Cron expression
  message = "Daily reminder"   # Message to send
  target  = ["U123", "C456"] # userID, channelID
}
```

### 7. Security

**Credentials**: `internal/slacker/security/credentials.go`

| Element | Line | Description |
|--------|------|-------------|
| `CredentialCrypto` struct | `:19` | AES-GCM encryption |
| `Initialize` | `:33` | Setup with passphrase |
| `EncryptCredentials` | `:80` | Encrypt credentials |
| `DecryptCredentials` | `:113` | Decrypt credentials |

**User Credentials**: `internal/slacker/security/user_credentials.go`

**Security Logging**: `internal/slacker/security/logger.go`

| Method | Description |
|--------|-------------|
| `LogError()` | Security errors |
| `LogToolRequest()` | Tool addition requests |
| `LogToolApprovalRequired()` | Approval requests |
| `LogToolApproval()` | Approval decisions |
| `LogSessionEvent()` | Session events |
| `LogAdminAction()` | Admin actions |
| `LogConnectionState()` | Connection state changes |

### 8. User Preferences

**Structure** (`internal/slacker/user_session.go:12-19`):

```go
type UserPreferences struct {
    ShowThinking   bool   `json:"show_thinking"`
    ShowTools      bool   `json:"show_tools"`
    ShowDone       bool   `json:"show_done"`
    ThinkingFormat string `json:"thinking_format"`
    ToolFormat     string `json:"tool_format"`
    Verbose        bool   `json:"verbose"`
}
```

**Defaults** (`internal/slacker/user_session.go:22-31`):
| Preference | Default | Description |
|------------|---------|-------------|
| `ShowThinking` | false | Display thinking process |
| `ShowTools` | true | Display tool invocations |
| `ShowDone` | true | Display completion messages |
| `ThinkingFormat` | "plain" | Thinking display format |
| `ToolFormat` | "detailed" | Tool display format |
| `Verbose` | false | Verbose output |

**Preferences Command**: `internal/slacker/commands/preferences.go`

### 9. Intelligent Help

When commands fail, the system can provide AI-powered assistance. This integration is in `internal/slacker/message_handler.go`.

Help is triggered when:
- Unknown command is entered
- Tool execution fails
- User requests help explicitly

## Model Access Control

**Configuration Block**: `internal/config/file.go:232-236`

```hcl
model_access {
  allowed_models = ["llama3.2:latest", "qwen2.5:7b"]
  denied_models = ["experimental:beta"]
}
```

**State File**: `slacker_state_path/model-access.json`

**Validation Logic**: `internal/config/file.go:421-477`

```
Validation Flow:
1. Check if user is admin → bypass all restrictions
2. Load state config (priority over HCL)
3. Check deny list (takes priority)
4. If allow list exists, verify model in list
5. Allow default model unconditionally
```

| Validation Function | Line |
|---------------------|------|
| `ValidateModelAccess()` | `:421` |
| `GetEffectiveModelAccess()` | `:479` |
| `SaveModelAccessState()` | `:380` |

**CLI vs Slacker Behavior**:
- **CLI Operations**: No model validation (bypass all restrictions)
- **Slacker Operations**: Full validation with allow/deny lists

## Connection Management

**Primary File**: `internal/slacker/connection.go`

| Element | Line | Description |
|--------|------|-------------|
| `SlackConnection` struct | `:17` | Slack API connection |
| `NewSlackConnection` | `:28` | Create connection |
| `StartSocketMode` | `:129` | Start Socket Mode |
| `ValidateSetup` | `:80` | Validate tokens |

**Socket Mode Events** (`internal/slacker/events.go:36-54`):
| Event | Handler |
|-------|---------|
| `socketmode.EventTypeHello` | `handleHello` |
| `socketmode.EventTypeConnecting` | `handleConnecting` |
| `socketmode.EventTypeConnected` | `handleConnected` |
| `socketmode.EventTypeConnectionError` | `handleConnectionError` |
| `socketmode.EventTypeEventsAPI` | `handleEventsAPI` |
| `socketmode.EventTypeInteractive` | `handleInteractive` |

## Configuration Example

Complete `marvin.hcl` configuration for Slacker:

```hcl
# LLM Configuration
model = "ministral-3:3b"
provider = "ollama"

# Multi-Tenant Configuration
multi_tenant {
  admin_users        = ["U1234567890"]
  admin_channel     = "CADMIN123"
  session_store_path = "./sessions"
  credential_store  = "./credentials"
  slacker_state_path = "./slacker-state"
  approval_timeout  = "24h"

  # Cron jobs
  cron {
    spec    = "0 9 * * *"
    message = "Daily standup reminder"
    target  = ["U1234567890", "CCHANNEL123"]
  }
}

# Model Access Control
model_access {
  allowed_models = ["llama3.2:latest", "qwen2.5:7b"]
  denied_models = ["experimental:beta"]
}

# Tool Configuration
local_program "web-fetch" {
  command = "/usr/bin/curl"
  args    = ["{{.args}}"]
}

docker_mcp "filesystem" {
  image = "meschbach/mcp-filesystem:latest"
  args  = ["/data"]
}

# Display Preferences
display {
  show_thinking = false
  show_tools   = true
  show_done    = true
}
```

## Component Dependencies

```
cmd/slacker/main.go
    │
    ▼
proc.Run()
    │
    ├──► config.Load()
    │       └──► internal/config/file.go
    │
    ├──► NewSlackBot()
    │       │
    │       ├──► NewSlackConnection()
    │       │       └──► internal/slacker/connection.go
    │       │
    │       ├──► NewSessionManager()
    │       │       └──► internal/slacker/session_manager.go
    │       │
    │       ├──► NewApprovalWorkflow()
    │       │       └──► internal/slacker/approval_workflow.go
    │       │
    │       ├──► NewSecurityLogger()
    │       │       └──► internal/slacker/security/logger.go
    │       │
    │       └──► TenantToolSet (query package)
    │
    └──► bot.StartSocketMode()
            └──► internal/slacker/bot.go
                    │
                    ├──► EventRouter.RouteEvent()
                    │       └──► internal/slacker/events.go
                    │
                    └──► MessageHandler.ProcessMessage()
                            │
                            ├──► Command matching
                            │       └──► internal/slacker/commands/
                            │
                            └──► Query processing
                                    └──► query_handler.go
```

## Error Handling

| Error Type | Location | Handling |
|-----------|----------|----------|
| Authentication failure | `connection.go:50` | Log error, exit |
| Tool initialization timeout | `message_handler.go:456` | Send timeout message to user |
| Session save failure | `session_manager.go:78` | Log warning, continue |
| Socket Mode disconnect | `bot.go:220` | Log error, attempt reconnect |
| Approval timeout | `approval_workflow.go:213` | Cleanup old approvals |

## Observability

**Tracing** is integrated via OpenTelemetry:

| Span | Location |
|------|----------|
| `MessageHandler.ProcessMessage` | `message_handler.go:341` |
| `MessageHandler.handleCommand` | `message_handler.go:494` |
| `MessageHandler.routeMessage` | `message_handler.go:574` |
| `MessageHandler.handleQuery` | `message_handler.go:600` |
| `slacker.SessionManager.GetOrCreateSession` | `session_manager.go:48` |
| `slacker.CronDispatcher.OnTrigger` | `cron_dispatcher.go:32` |

**Attributes**:
- `channel` - Channel ID
- `user` - User ID
- `command.matched` - Matched command
- `queryProcessor.initialized` - Tool initialization status