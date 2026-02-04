# Slacker Bot Configuration Example
# This file demonstrates how to configure the multi-tenant Slack bot using Socket Mode
# 
# Socket Mode Requirements:
# 1. Create a Slack App at https://api.slack.com/apps
# 2. Enable Socket Mode in the app settings
# 3. Add the Required Bot Token Scopes:
#    - Basic: bot, chat:write, app_mentions:read, channels:history
#    - Presence: users:read, users:write, presence:write (required for bot to appear online)
# 4. Add Required Event Subscriptions: message.channels, app_mention
# 5. Generate App Token (xapp-...) with socket:write scope
# 6. Generate Bot Token (xoxb-...) with all required scopes
#
# Environment Variables Required:
# - SLACK_BOT_TOKEN: Bot token (xoxb-...)  
# - SLACK_APP_TOKEN: App token for Socket Mode (xapp-...)

model = "ministral-3:3b"

# Multi-tenant settings
multi_tenant {
  admin_users = ["U1234567890", "U0987654321"]  # Replace with actual Slack user IDs
  admin_channel = "CADMIN"  # Optional admin notification channel
  session_store_path = "./sessions"
  credential_store = "./credentials"
}

# Global HTTP MCP tools (available to all users, no approval required)
mcp_over_http "weather-api" {
  name = "weather-api"
  url = "https://weather.example.com/mcp"
}

mcp_over_http "stock-prices" {
  name = "stock-api"
  url = "https://api.stocks.com/mcp"
}

# Pre-approved shared tools (admin-configured, available to specific users)
local_program "company-git" {
  name = "company-git"
  program = "/usr/local/bin/git-mcp"
  args = ["--read-only", "--company-repo"]
  
  sharing {
    allowed_users = ["U1234567890", "U0987654321"]
    can_share = false
    expires_at = "2025-12-31T23:59:59Z"
  }
}

docker_mcp "shared-docker" {
  name = "shared-docker"
  image = "company/mcp-tools:latest"
  
  env {
    key = "COMPANY_API_KEY"
    pass_through = true  # Use environment variable from system
  }
  
  sharing {
    allowed_users = ["U1234567890"]
    can_share = false
  }
}

# System prompt for the AI
system_prompt {
  from_string = <<EOS
You are a helpful AI assistant integrated with Slack. You have access to various tools to help users with their tasks.

When users ask you to add tools:
- HTTP/MCP tools can be added immediately
- Local programs and Docker tools require admin approval

Always be helpful, professional, and clear about what you're doing. If you need to make tool calls, explain what you're doing first.
EOS
}