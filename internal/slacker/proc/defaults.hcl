# Slacker Bot Configuration
# This uses Socket Mode API (recommended for modern Slack apps)
#
# Socket Mode Requirements:
# 1. Create a Slack App at https://api.slack.com/apps
# 2. Enable Socket Mode in the app settings  
# 3. Add the Bot User scope (bot, chat:write, etc.)
# 4. Add Event Subscriptions: message.channels, app_mention
# 5. Generate App Token (xapp-...) with socket:write scope
# 6. Generate Bot Token (xoxb-...) with required scopes
#
# Environment Variables Required:
# - SLACK_BOT_TOKEN: Bot token (xoxb-...)  
# - SLACK_APP_TOKEN: App token for Socket Mode (xapp-...)
#
# Start the bot with:
# Method 1: Direct passphrase (less secure)
# ./slacker --config marvin.slacker.hcl --passphrase your-secret
#
# Method 2: Passphrase file (recommended for production)
# ./slacker --config marvin.slacker.hcl --passphrase-file /path/to/passphrase.txt
#
# Note: Passphrase files with permissive permissions will show a warning but continue.

model = "ministral-3:3b"

# Advanced model options for fine-tuning responses
options {
  # Context window size (tokens) - default: 2048
  context_window_size = 4096
  
  # Sampling temperature (0.0-1.0) - higher = more creative - default: 0.8
  temperature = 0.7
  
  # Nucleus sampling (0.0-1.0) - default: 0.9
  top_p = 0.9
  
  # Top-k sampling (-1 = no limit) - default: 40
  top_k = 40
  
  # Maximum tokens in response (-1 = unlimited) - default: -1
  num_predict = 2048
  
  # Repetition penalty - default: 1.1
  repeat_penalty = 1.1
  
  # How far back to check for repetitions (0 = disabled) - default: 64
  repeat_last_n = 64
  
  # Random seed for reproducible results (optional)
  seed = 42
  
  # Stop sequences to end generation (optional)
  stop = ["###", "END"]
}

# Multi-tenant settings
multi_tenant {
  admin_users = ["YOUR_ADMIN_USER_ID_HERE"]
  admin_channel = "C_ADMIN_CHANNEL_ID_HERE"  # Optional admin notification channel
  session_store_path = "./sessions"
  credential_store = "./credentials"
}

# Global HTTP MCP tools (available to all users, no approval required)
mcp_over_http "weather-api" {
  name = "weather-api"
  url = "https://weather.example.com/mcp"
}

# Pre-approved shared tools (admin-configured)
local_program "company-git" {
  name = "company-git"
  program = "/usr/local/bin/git-mcp"
  args = ["--read-only"]
  
  sharing {
    allowed_users = ["USER_ID_HERE"]
    can_share = false
  }
}

docker_mcp "shared-docker" {
  name = "shared-docker"
  image = "your-docker-tool:latest"
  
  sharing {
    allowed_users = ["USER_ID_HERE"]
    can_share = false
  }
}
