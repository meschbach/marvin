# Slacker - Multi-Tenant Slack Bot

Slacker is a multi-tenant Slack bot built on Marvin's architecture that provides AI-powered tool management with
per-user isolation and admin approval workflows.

## Features

- **Multi-tenant Architecture**: Each user has isolated sessions and tool access
- **Natural Language Tool Management**: Add, list, and share tools using natural language
- **Admin Approval Workflow**: Local and Docker MCP servers require admin approval
- **HTTP MCP Tools**: Instant access without approval required
- **Session Persistence**: JSON-based session storage with conversation history
- **Credential Encryption**: AES-GCM encryption for user credentials
- **Security Logging**: All security events logged to stdout

## Quick Start

### 1. Build the Bot

```bash
go build -o slacker ./cmd/slacker
```

### 2. Configure the Bot

Copy the example configuration:
```bash
cp marvin.slacker.example.hcl marvin.slacker.hcl
```

Edit `marvin.slacker.hcl` with your settings:
- Replace `admin_users` with actual Slack user IDs
- Configure your preferred MCP tools
- Set storage paths for sessions and credentials

### 3. Set Up Slack Bot Token

Create a Slack Bot and get the Bot User OAuth Token:
```bash
export SLACK_BOT_TOKEN=xoxb-your-bot-token-here
```

### 4. Run the Bot

```bash
./slacker --config marvin.slacker.hcl --passphrase "your-secure-passphrase"
```

## User Interactions

### Adding HTTP MCP Tools (Instant Access)

```
User: @slacker Add HTTP MCP server at https://api.github.com/mcp
Bot: ✅ Added "github-api" HTTP MCP server successfully. Available tools:
     • github-api.search_repositories
     • github-api.get_file_content
     • github-api.create_issue
```

### Requesting Local/Docker Tools (Requires Approval)

```
User: @slacker Add local program at /usr/local/bin/my-tool
Bot: 📋 Tool approval request submitted:
     • Tool ID: local-req-12345
     • Status: Pending approval
     • I'll notify you when it's approved.
```

Admins receive DM notifications:
```
Admin: 🔧 **Tool Approval Required**

**Requester:** <@U1234567890>
**Tool Type:** local_program
**Tool Name:** my-tool
**Request ID:** local-req-12345

**To approve:** Reply with "Approve local-req-12345"
**To reject:** Reply with "Reject local-req-12345: security policy"
```

### Listing Available Tools

```
User: @slacker List my tools
Bot: 🔧 **Your Available Tools:**

1. `github-api.search_repositories`
2. `github-api.get_file_content`
3. `weather-api.get_current`
4. `weather-api.get_forecast`
```

### Tool Sharing

```
User: @slacker Share my database-tool with @jane.doe
Bot: ✅ Database tool shared with @jane.doe. She can now use it in her conversations.
```

## Configuration

### Basic Configuration

```hcl
model = "ministral-3:3b"

multi_tenant {
  admin_users = ["U1234567890"]
  session_store_path = "./sessions"
  credential_store = "./credentials"
}
```

### Tool Configuration

#### HTTP MCP Tools (No Approval Required)
```hcl
mcp_over_http "weather-api" {
  name = "weather-api"
  url = "https://weather.example.com/mcp"
}
```

#### Local Programs (Approval Required)
```hcl
local_program "company-tool" {
  name = "company-tool"
  program = "/usr/local/bin/company-mcp"
  args = ["--read-only"]

  sharing {
    allowed_users = ["U1234567890"]
    can_share = false
  }
}
```

#### Docker Tools (Approval Required)
```hcl
docker_mcp "shared-docker" {
  name = "shared-docker"
  image = "company/tools:latest"

  env {
    key = "API_KEY"
    value = "your-api-key"
  }

  sharing {
    allowed_users = ["U1234567890"]
    can_share = true
  }
}
```

## Security Features

### Access Control
- **Admin Users**: Can approve/reject tools and access all tools
- **Regular Users**: Can add HTTP tools, need approval for local/Docker tools
- **Tool Sharing**: Users can share tools if permitted by admin

### Credential Management
- AES-GCM encryption with Argon2 key derivation
- Per-user encrypted credential files
- Passphrase-protected key storage

### Security Logging
All security events are logged to stdout:
```
[SECURITY] 2025-01-15T10:30:45Z - Tool Request - User: U123456, Type: http, URL: https://api.github.com/mcp
[SECURITY] 2025-01-15T10:30:46Z - Tool Added - User: U123456, ToolID: user_U123456_github-api, Type: http
[SECURITY] 2025-01-15T10:31:20Z - Approval Required - ToolID: local-req-12345, Requester: U123456, Type: local
[SECURITY] 2025-01-15T10:32:10Z - Tool Approval - Admin: U789012, ToolID: local-req-12345, Decision: approved
```

## Architecture

### Session Management
- JSON files stored in configured directory
- `session_{userID}_{channelID}.json` format
- Conversation history and available tools per session

### Tool Isolation
- Per-user `ToolSet` instances
- Namespaced tool names: `user_{userID}_{toolName}`
- Admin-approved global tools available to all

### Approval Workflow
- HTTP tools: Instant access
- Local/Docker tools: Admin approval required
- DM notifications to all admin users
- Approve/reject via natural language replies

## Deployment

### Environment Variables
- `SLACK_BOT_TOKEN`: Bot User OAuth Token (required)

### Command Line Options
```
-config string     Path to configuration file (default "marvin.slacker.hcl")
-sessions string   Directory to store sessions (default "./sessions")
-credentials string Directory to store encrypted credentials (default "./credentials")
-passphrase string  Passphrase for credential encryption (required)
-verbose          Enable verbose logging
```

### Docker Deployment (Optional)

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o slacker ./cmd/slacker

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/slacker .
CMD ["./slacker", "--config", "marvin.slacker.hcl"]
```

## Troubleshooting

### Common Issues

1. **"SLACK_BOT_TOKEN environment variable is required"**
   - Set the environment variable with your bot token
   - Ensure that token has required scopes

2. **"Error loading configuration"**
   - Check the configuration file syntax
   - Ensure file paths are correct

3. **"Tool approval not working"**
   - Verify admin user IDs are correct
   - Check bot has permission to send DMs to admins

4. **"Panic: Failed to save salt: no such file or directory"** (Fixed!)
   - This happened when credential directories didn't exist on first run
   - Application now creates directories automatically before attempting file operations
   - No more panic - graceful error handling implemented

5. **"Configuration paths not respected"** (Fixed!)
   - Session store and credential store paths now respect config file settings
   - Command line options override config file values
   - Directories are created automatically if they don't exist

### Debug Mode

Run with verbose logging:
```bash
./slacker --verbose --config marvin.slacker.hcl --passphrase "your-passphrase"
```

### First Run Setup

On first run with default paths:
```bash
# Application will automatically create:
./sessions/          # Session storage
./credentials/        # Credential storage
./credentials/.key.salt  # Salt file for encryption
```

### Enhanced Diagnostics (Implemented)

The enhanced Slacker bot now provides comprehensive logging and diagnostics:

#### Connection and Startup Logging
```
[DIAGNOSTIC] 2025-01-15T10:30:45Z - Bot Connected - BotID: U9876543210, User: slacker-bot, Team: T12345678/My-Team
[DIAGNOSTIC] 2025-01-15T10:30:46Z - Configuration loaded successfully
[DIAGNOSTIC] 2025-01-15T10:30:46Z - Session State - User: U123456, Channel: C123456, Total Sessions: 1
```

#### Security Event Logging
All security events are now logged with timestamps and user context:
```
[SECURITY] 2025-01-15T10:35:00Z - Tool Request - User: U123456, Type: http, URL: https://api.github.com/mcp
[SECURITY] 2025-01-15T10:35:02Z - Tool Added - User: U123456, ToolID: user_U123456_github-api, Type: http
[SECURITY] 2025-01-15T10:31:20Z - Approval Required - ToolID: local-req-12345
[SECURITY] 2025-01-15T10:32:10Z - Tool Approval - Admin: U789012, ToolID: local-req-12345, Decision: approved
```

#### Directory Management
Directories are created automatically with proper error handling:
- Sessions directory (0755 permissions)
- Credentials directory (0755 permissions)
- Salt file generation with secure permissions

### Known Issues

#### LSP Caching
There may be occasional LSP (Language Server Protocol) caching issues after multiple file edits. These do not affect
the build or runtime functionality. If experiencing LSP errors:
1. Run `go clean -cache` to clear the cache
2. Restart your IDE/editor

#### Build System
The application builds successfully:
```bash
go build -o slacker ./cmd/slacker
```

### Debug Mode

Run with verbose logging for troubleshooting:
```bash
./slacker --verbose --config marvin.slacker.hcl --passphrase "your-passphrase"
```

### First Run Setup

On first run with default paths:
```bash
# Application will automatically create:
./sessions/          # Session storage
./credentials/        # Credential storage
./credentials/.key.salt  # Salt file for encryption
```

## Development

### Project Structure
```
cmd/slacker/           # Main application entry point
internal/slacker/      # Core Slack bot functionality
internal/slacker/security/  # Credential management and logging
internal/query/       # Multi-tenant tool management
internal/config/      # Configuration handling
```

### Building
```bash
# Build for current platform
go build -o slacker ./cmd/slacker

# Build for multiple platforms
GOOS=linux GOARCH=amd64 go build -o slacker-linux ./cmd/slacker
GOOS=darwin GOARCH=amd64 go build -o slacker-macos ./cmd/slacker
```

### Testing
```bash
# Run tests
go test ./internal/slacker/...

# Run integration tests
go test ./... -tags=integration
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## License

This project is part of the Marvin AI workbench and follows the same licensing terms.
