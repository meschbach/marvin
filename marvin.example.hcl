model = "llama3.2:latest"

# LLM Provider: "ollama" (default), "openrouter", or "gemini"
# provider = "ollama"

# OpenRouter configuration (when provider = "openrouter")
# The API key can be provided via:
#   1. openrouter block (api_key)
#   2. --openrouter-key-file CLI flag
#   3. OPENROUTER_API_KEY environment variable
#
# openrouter {
#   # API key directly (not recommended for production)
#   # api_key = "sk-or-v1-..."
#   
#   # Path to file containing API key (recommended)
#   # api_key_file = "/path/to/openrouter-api-key.txt"
#   
#   # Optional: Override default OpenRouter endpoint
#   # base_url = "https://openrouter.ai/api/v1"
#   
#   # Optional: Retry configuration for handling rate limits
#   # retry {
#   #   # Maximum retry attempts (default: 3)
#   #   max_attempts = 3
#   #   
#   #   # Initial backoff interval (default: 1s)
#   #   initial_interval = "1s"
#   #   
#   #   # Maximum backoff interval (default: 30s)
#   #   max_interval = "30s"
#   #   
#   #   # Maximum wait time for rate limit reset (default: 2m)
#   #   # The server's X-RateLimit-Reset header specifies reset time in milliseconds
#   #   max_rate_limit_wait = "2m"
#   # }
# }

# Example OpenRouter models:
# provider = "openrouter"
# model = "qwen2.5:7b"
# or
# model = "openrouter/qwen2.5:7b"

# Gemini configuration (when provider = "gemini")
# The API key can be provided via:
#   1. gemini block (api_key)
#   2. gemini block (api_key_file)
#   3. GEMINI_API_KEY environment variable
#
# gemini {
#   # API key directly (not recommended for production)
#   # api_key = "AIza..."
#   
#   # Path to file containing API key (recommended)
#   # api_key_file = "/path/to/gemini-api-key.txt"
# }

# Example Gemini models:
# provider = "gemini"
# model = "gemini-2.0-flash"
# or
# model = "gemini-1.5-pro"

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

system_prompt {
  from_string = <<EOS
You are an example AI tool demonstrating Marvin, an AI tooling assistant allowing for plugging in various assistants
within knowledge bases.
EOS
}

# Display preferences for output formatting
display {
  # Whether to show AI thinking process (default: false)
  show_thinking = true
  
  # Format for displaying thinking content: "plain", "markdown", or "collapsed" (default: "plain")
  thinking_format = "markdown"
  
  # Whether to show tool invocation details (default: true)
  show_tools = true
  
  # Format for displaying tool details: "simple" or "detailed" (default: "detailed")
  tool_format = "detailed"
  
  # Whether to show completion messages (default: true)
  show_done = true
  
  # Whether to enable verbose debugging output (default: false)
  verbose = false
}

local_program "gitea" {
  program = "/opt/homebrew/bin/gitea-mcp-server"
  args = ["-host", "https://gitea.example.com", "-read-only","-token", "<some token>"]
  
  assistant_prompt {
    from_string = <<EOS
You have access to a Gitea instance via MCP. Use this for:
- Repository management (create, clone, list)
- Issue and pull request operations
- Code review and collaboration
- User and organization management
Best practices: Use read-only mode when viewing data, be cautious with write operations.
EOS
  }
}

local_program "docs" {
  # provides access to specific files
  program = "file-mcp"
  
  assistant_prompt {
    from_string = <<EOS
You have access to file system operations through the file-mcp tool. Use this for:
- Reading and writing files
- Directory navigation and listing
- File search operations
- Basic file system management
Best practices: Always verify file paths before operations, be careful with destructive operations.
EOS
  }
}

# Example Docker-based MCP server with assistant prompt
docker_mcp "postgres" "postgres:15" {
  
  assistant_prompt {
    from_string = <<EOS
You have access to a PostgreSQL database running in Docker. Capabilities:
- SQL queries and data manipulation
- Schema inspection and management
- Database administration tasks
Best practices: Use transactions for multi-step operations, validate inputs before execution.
EOS
  }
}

# Example HTTP-based MCP server with assistant prompt  
mcp_over_http "weather_api" "http://weather-api:8080" {
  
  assistant_prompt {
    from_string = <<EOS
You have access to weather data via HTTP API. Use this for:
- Current weather conditions
- Weather forecasts
- Historical weather data
Usage: Always specify location and date range when applicable.
EOS
  }
}

# Multi-tenant configuration for Slack/Slacker operations
multi_tenant {
    # List of admin user IDs who can bypass all restrictions
    admin_users = ["U123456789", "U987654321"]
    
    # Directory for storing user sessions
    session_store_path = "./sessions"
    
    # Directory for storing user credentials
    credential_store = "./credentials"
    
    # Dedicated directory for Slacker-only state (NEW)
    slacker_state_path = "./slacker-state"
    
    # Optional: Custom security log format
    # security_log_format = "[SECURITY] %s - %s - User: %s - %s"
    
    # Optional: Tool approval timeout
    # approval_timeout = "24h"
    
    # Optional: Admin channel for notifications
    # admin_channel = "#admin-alerts"
}

# Model access control for Slack/Slacker operations (NEW)
model_access {
    # List of explicitly allowed models (empty = allow all except denied)
    allowed_models = ["llama3.2:latest", "qwen2.5:7b", "mistral:latest"]
    
    # List of explicitly denied models (always blocked)
    denied_models = ["experimental:beta", "test-model:unstable"]
}
