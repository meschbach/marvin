# Slacker Setup Guide

This guide walks you through setting up Slacker, Marvin's multi-tenant Slack bot with AI tool management and admin
approval workflows.

## 🎯 **Overview**

Slacker provides:
- **Multi-tenant architecture**: Each user has isolated sessions and tool access
- **Natural language tool management**: Add, list, and share tools via conversation
- **Admin approval workflows**: Security controls for sensitive tool types
- **Enterprise-grade security**: Encrypted credentials, audit logging

## 📋 **Prerequisites**

- **Slack Workspace**: Admin access to create apps
- **Go 1.25+**: If building from source
- **Docker** (optional): For container deployment
- **Ollama server**: For LLM functionality (can be remote)

---

## 🚀 **Step 1: Create Slack App**

### 1.1 Create New App
1. Go to [Slack API](https://api.slack.com/apps)
2. Click **"Create New App"**
3. Choose **"From scratch"**
4. Enter app details:
   - **App Name**: `Marvin Slacker` (or your preferred name)
   - **Development Workspace**: Select your workspace
5. Click **"Create App"**

### 1.2 Configure Bot Permissions

#### **Bot Token Scopes**
Navigate to **"OAuth & Permissions"** → **"Bot Token Scopes"** and add:

**Required Scopes:**
```
app_mentions:read          # Read @mentions
channels:history          # Read channel history
chat:write               # Send messages
commands                 # Handle slash commands
files:read               # Read file uploads
files:write              # Upload files
im:history              # Read DM history
im:write                # Send DMs
users:read              # Read user information
users:write              # Update user presence
presence:write          # Set bot presence (appears online)
```

**Optional Scopes (for advanced features):**
```
admin                 # Workspace admin operations (if bot is admin)
channels:manage        # Manage channel settings
groups:history        # Read private channel history
groups:write          # Post in private channels
```

### 1.3 Enable Socket Mode

1. Navigate to **"Socket Mode"**
2. Toggle **"Enable Socket Mode"** to **On**
3. Under **"App-Level Tokens"**, click **"Generate Token and Scopes"**
4. Enter token name: `socket-mode-token`
5. Add scope: `connections:write`
6. Click **"Generate"**
7. **Save the token** (starts with `xapp-`)

### 1.4 Configure Event Subscriptions

1. Navigate to **"Event Subscriptions"**
2. Toggle **"Enable Events"** to **On**
3. Under **"Subscribe to bot events"**, add:
   ```
   app_mention           # When bot is mentioned
   message.channels      # Messages in channels bot is in
   message.im           # Direct messages
   ```

4. Click **"Save Changes"**

### 1.5 Install App to Workspace

1. Navigate to **"Install App"**
2. Click **"Install to Workspace"**
3. Authorize the required permissions
4. **Save the Bot User OAuth Token** (starts with `xoxb-`)

---

## ⚙️ **Step 2: Configure Slacker**

### 2.1 Build/Download Slacker

#### **Option A: Build from Source**
```bash
# Clone repository
git clone https://github.com/meschbach/marvin.git
cd marvin

# Build Slacker
go build -o slacker ./cmd/slacker
```

#### **Option B: Download Pre-built Binary**
```bash
# Download for your platform
curl -L https://github.com/meschbach/marvin/releases/latest/download/slacker_linux_amd64.tgz | tar xz
chmod +x slacker
```

#### **Option C: Use Docker**
```bash
docker pull ghcr.io/meschbach/marvin/slacker:latest
```

### 2.2 Create Configuration File

Copy the example configuration:
```bash
cp marvin.slacker.example.hcl marvin.slacker.hcl
```

Edit `marvin.slacker.hcl` with your settings:

```hcl
# Model configuration
model = "ministral-3:3b"

# Multi-tenant settings
multi_tenant {
  # Replace with your admin user IDs (get from Slack: right-click user → "Copy member ID")
  admin_users = ["U1234567890", "U0987654321"]

  # Optional admin channel for notifications
  admin_channel = "CADMIN123"

  # Storage paths
  session_store_path = "./sessions"
  credential_store = "./credentials"
}

# System prompt
system_prompt {
  from_string = <<EOS
You are a helpful AI assistant integrated with Slack...
EOS
}

# Intelligent help system (NEW!)
help_system {
  enabled = true
  confidence_threshold = 0.7      # Trigger help below this confidence
  model = "ministral-3:3b"         # Model for help analysis
  max_context_messages = 5         # Messages to consider for context
  analysis_timeout = 5             # Timeout in seconds

  # Enable help for specific scenarios
  help_on_intent_failure = true           # Command recognition help
  help_on_model_access_denied = true      # Model access help
  help_on_tool_configuration_error = true # Tool setup help
  help_on_tool_permission_denied = true   # Permission help
}
```

**Getting User IDs:**
1. In Slack, right-click on a user
2. Select **"Copy member ID"**
3. Paste into `admin_users` array

### 2.3 Set Environment Variables

```bash
# Required environment variables
export SLACK_BOT_TOKEN=xoxb-your-bot-token-here
export SLACK_APP_TOKEN=xapp-your-app-token-here

# Optional: For credential encryption
export SLACKER_PASSPHRASE=your-secure-passphrase
```

**Security Note:**
- Store tokens securely (use environment variables, not files)
- Use a strong passphrase for credential encryption
- Consider using a secret management system in production

---

## 🏃 **Step 3: Run Slacker**

### 3.1 Basic Execution

```bash
# Run with default settings
./slacker --config marvin.slacker.hcl

# Run with explicit passphrase
./slacker --config marvin.slacker.hcl --passphrase "your-secure-passphrase"

# Run with custom storage paths
./slacker --config marvin.slacker.hcl --sessions ./my-sessions --credentials ./my-credentials
```

### 3.2 Docker Execution

```bash
# Create volumes for persistence
mkdir -p sessions credentials

# Run with Docker
docker run --rm \
  -v $(pwd)/marvin.slacker.hcl:/config/marvin.slacker.hcl \
  -v $(pwd)/sessions:/sessions \
  -v $(pwd)/credentials:/credentials \
  -e SLACK_BOT_TOKEN=xoxb-your-token \
  -e SLACK_APP_TOKEN=xapp-your-token \
  -e SLACKER_PASSPHRASE=your-passphrase \
  ghcr.io/meschbach/marvin/slacker:latest \
  --config /config/marvin.slacker.hcl
```

### 3.3 Verify Connection

When Slacker starts successfully, you should see:
```
[INFO] Slacker starting...
[DIAGNOSTIC] Bot Connected - BotID: U9876543210, User: marvin-bot, Team: T12345678/Your-Team
[INFO] Ready to handle messages
```

In Slack, the bot should appear **"Online"** in the workspace.

---

## 🧪 **Step 4: Test Basic Functionality**

### 4.1 Invite Bot to Channel
1. In any channel, type `/invite @marvin-bot`
2. Or add bot via channel settings → **"Integrations"** → **"Add apps"**

### 4.2 Test Basic Interaction
1. Mention the bot: `@marvin-bot hello`
2. The bot should respond with a greeting

### 4.3 Test Tool Management
1. Add an HTTP tool: `@marvin-bot Add HTTP MCP server at https://weather.example.com/mcp`
2. Bot should confirm: `✅ Added "weather-api" HTTP MCP server successfully`

### 4.4 Test Approval Workflow
1. Request a local tool: `@marvin-bot Add local tool at /usr/bin/ls`
2. Bot should respond with approval request details
3. Admins should receive DM notifications

---

## 🔧 **Step 5: Configure Ollama Integration**

### 5.1 Local Ollama (Default)
```bash
# Ensure Ollama is running
ollama serve

# Pull required model
ollama pull ministral-3:3b
```

### 5.2 Remote Ollama
```bash
# Set Ollama host
export OLLAMA_HOST=http://your-ollama-server:11434

# Or in configuration file
# Add to marvin.slacker.hcl:
# ollama_host = "http://your-ollama-server:11434"
```

### 5.3 Model Selection
Edit your configuration to use different models:

```hcl
# Available models (example)
model = "qwen3:8b"           # Larger context window
model = "llama3.2:latest"     # Good reasoning
model = "mistral:latest"      # Fast responses
```

---

## 📊 **Step 6: Monitoring and Logging**

### 6.1 Enable Verbose Logging
```bash
./slacker --config marvin.slacker.hcl --verbose
```

### 6.2 Log Output
Slacker outputs structured logs:
```
[DIAGNOSTIC] 2026-02-10T15:30:45Z - Bot Connected - BotID: U9876543210, User: marvin-bot, Team: T12345678/Your-Team
[SECURITY] 2026-02-10T15:31:20Z - Tool Request - User: U123456, Type: http, URL: https://api.github.com/mcp
[INFO] 2026-02-10T15:32:10Z - Tool Approval - Admin: U789012, ToolID: local-req-12345, Decision: approved
```

### 6.3 Health Checks
Monitor bot health by checking:
- Bot presence in Slack (should be "Online")
- Response to `@marvin-bot ping`
- Log output for errors
- Ollama connectivity

---

## 🚨 **Troubleshooting**

### **Common Issues**

#### **"SLACK_BOT_TOKEN environment variable is required"**
```bash
# Set the environment variable
export SLACK_BOT_TOKEN=xoxb-your-bot-token-here

# Verify token format (should start with xoxb-)
echo $SLACK_BOT_TOKEN
```

#### **Bot appears offline**
1. Check Socket Mode is enabled in app settings
2. Verify `SLACK_APP_TOKEN` is set and valid
3. Check bot has `presence:write` scope
4. Review logs for connection errors

#### **No response to mentions**
1. Verify bot is in the channel (`/invite @marvin-bot`)
2. Check bot has `app_mentions:read` and `message.channels` scopes
3. Ensure Event Subscriptions are configured correctly
4. Check firewall/proxy settings

#### **"Error loading configuration"**
1. Verify HCL syntax is correct
2. Check file paths and permissions
3. Ensure admin user IDs are valid Slack user IDs
4. Run with `--verbose` for detailed error information

#### **Ollama connection issues**
1. Check `OLLAMA_HOST` environment variable
2. Verify Ollama is running: `ollama list`
3. Test connectivity: `curl http://localhost:11434/api/tags`
4. Ensure required model is pulled: `ollama pull ministral-3:3b`

### **Debug Mode**
Run with maximum debugging:
```bash
./slacker --config marvin.slacker.hcl --verbose --sessions ./debug-sessions
```

### **Log Files**
Check application logs for:
- Connection status
- Error messages
- Security events
- Tool management actions

---

## 🎯 **Next Steps**

Once Slacker is running successfully:

1. **[Intelligent Help System](intelligent-help.md)**: 🆕 Learn about AI-powered help when commands fail
2. **[Admin Guide](admin-guide.md)**: Learn about user management and approval workflows
3. **[Security Guide](security.md)**: Implement security best practices
4. **[Tool Management Guide](tools-management.md)**: Advanced tool configuration and sharing
5. **[Kubernetes Deployment](../deployment/kubernetes.md)**: Deploy Slacker in production

## 📚 **Additional Resources**

- [Slack API Documentation](https://api.slack.com/)
- [Socket Mode Documentation](https://api.slack.com/apps/connections)
- [MCP Protocol Documentation](https://modelcontextprotocol.io/)
- [Marvin CLI Documentation](../../README.md)

---

## 🆘 **Getting Help**

If you encounter issues:

1. **Check logs** for specific error messages
2. **Verify configuration** syntax and values
3. **Test Slack app** settings and permissions
4. **Review troubleshooting** section above
5. **Open an issue** on [GitHub](https://github.com/meschbach/marvin/issues)

Remember: Slacker requires careful attention to security settings and permissions. Take time to verify each step before proceeding to production deployment.