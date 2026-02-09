model = "llama3.2:latest"

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
