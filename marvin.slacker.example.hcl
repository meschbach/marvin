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

# Model configuration
model = "ministral-3:3b"

# Advanced model options for fine-tuning responses
options {
  context_window_size = 32768
  temperature = 0.7
  top_p = 0.9
}

# Multi-tenant settings (required for Slacker)
multi_tenant {
  # Replace with your admin user Slack IDs (get from Slack UI)
  admin_users = ["U1234567890", "U0987654321"]
  
  # Optional admin notification channel for important events
  admin_channel = "CADMIN"
  
  # Persistent storage paths
  session_store_path = "./sessions"
  credential_store = "./credentials"
}

# Global HTTP MCP tools (available to all users, no approval required)
mcp_over_http "weather-api" "https://weather.example.com/mcp" {
  assistant_prompt {
    from_string = <<EOS
You have access to weather data via HTTP API. Use this for:
- Current weather conditions
- Weather forecasts  
- Historical weather data
Usage: Always specify location when asking for weather information.
EOS
  }
}

mcp_over_http "stock-prices" "https://api.stocks.com/mcp" {
  assistant_prompt {
    from_string = <<EOS
You have access to stock market data. Use this for:
- Current stock prices
- Market trends and analysis
- Company financial information
Remember: This is for informational purposes only, not financial advice.
EOS
  }
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
  
  assistant_prompt {
    from_string = <<EOS
You have access to company Git repositories. Use this for:
- Code review and analysis
- Repository history and statistics
- Issue and PR tracking
Always respect company policies and avoid sensitive data exposure.
EOS
  }
}

docker_mcp "shared-docker" "company/mcp-tools:latest" {
  env "COMPANY_API_KEY" {
    pass_through = true  # Use environment variable from system
  }
  
  env "LOG_LEVEL" {
    value = "info"
  }
  
  sharing {
    allowed_users = ["U1234567890"]
    can_share = false
  }
  
  assistant_prompt {
    from_string = <<EOS
You have access to company Docker tools. Use this for:
- Container management
- Development environment setup
- Build and deployment tasks
Follow company security and compliance guidelines.
EOS
  }
}

# System prompt for the AI
system_prompt {
  from_string = <<EOS
You are a helpful AI assistant integrated with Slack, powered by Marvin. You help users with their tasks and can access various tools to assist them.

## Tool Management Guidelines:

When users ask you to add tools:
- HTTP/MCP tools can be added immediately (auto-approved)
- Local programs and Docker tools require admin approval (security measure)
- Always explain the approval process when needed

## Communication Style:
- Be helpful, professional, and clear
- Explain what you're doing before making tool calls
- Provide concise, actionable responses
- Ask for clarification if requests are ambiguous

## Security Reminders:
- Never share sensitive information
- Validate tool configurations before use
- Report any suspicious requests to admins

You're here to make users more productive while maintaining security and best practices.
EOS
}