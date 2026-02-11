# Complete Configuration Reference

## Configuration File Structure

Marvin uses HCL configuration files. Default locations:
- `.marvin.hcl` (primary)
- `marvin.test.hcl` (testing)
- `-c <file>` (custom path)

## Model Configuration

### Ollama Models
```hcl
model "ollama" "main" {
  name     = "ministral-3:3b"
  host     = "http://localhost:11434"
  timeout  = "60s"
  max_tokens = 4096
}

model "ollama" "large" {
  name     = "llama3.1:8b"
  host     = "http://ollama.company.com:11434"
  timeout  = "120s"
  temperature = 0.7
}
```

### OpenAI Models
```hcl
model "openai" "gpt4" {
  api_key  = var.openai_api_key
  model    = "gpt-4"
  base_url = "https://api.openai.com/v1"
  timeout  = "90s"
}
```

### Model Parameters
- `name`: Model identifier
- `host`: API endpoint URL
- `timeout`: Request timeout duration
- `max_tokens`: Response token limit
- `temperature`: Randomness (0.0-1.0)
- `top_p`: Nucleus sampling (0.0-1.0)

## Program Configuration

### Basic Program
```hcl
program "marvin" "default" {
  model        = model.ollama.main
  description  = "General purpose assistant"
  system_prompt = "You are a helpful AI assistant."
}
```

### Program with Tools
```hcl
program "marvin" "docker" {
  model       = model.ollama.main
  description = "Container management assistant"
  
  tool "docker" {
    executable = "docker"
    args       = ["ps", "-a"]
  }
  
  tool "kubectl" {
    executable = "kubectl"
    args       = ["get", "pods"]
  }
}
```

### Program with Filesystem Access
```hcl
program "marvin" "fileops" {
  model = model.ollama.large
  
  filesystem {
    read  = ["/tmp", "./src"]
    write = ["/tmp/output"]
    exec  = false
  }
}
```

## MCP Tool Configuration

### Docker MCP
```hcl
mcp "docker" {
  command = "docker-mcp-server"
  args    = ["--socket", "/var/run/docker.sock"]
  
  config = {
    allowed_images = ["ubuntu:*", "alpine:*"]
    network_access = false
  }
}
```

### Filesystem MCP
```hcl
mcp "filesystem" {
  command = "filesystem-mcp-server"
  args    = ["/workspace"]
  
  config = {
    read_only_paths  = ["/src", "/docs"]
    read_write_paths = ["/tmp"]
    deny_paths       = ["/etc", "/var/secrets"]
  }
}
```

### HTTP MCP
```hcl
mcp "http" {
  command = "http-mcp-server"
  
  config = {
    allowed_domains = ["api.company.com", "github.com"]
    timeout = "30s"
    max_response_size = "10MB"
  }
}
```

### Google Workspace MCP
```hcl
mcp "google-workspace" {
  command = "google-workspace-mcp-server"
  args    = ["--config", "/config/gcp.json"]
  
  config = {
    credentials_file = "/path/to/credentials.json"
    scopes = [
      "https://www.googleapis.com/auth/drive.readonly",
      "https://www.googleapis.com/auth/gmail.readonly"
    ]
  }
}
```

## Slack Configuration

### Basic Bot Setup
```hcl
slack {
  bot_token      = var.slack_bot_token
  app_token      = var.slack_app_token
  signing_secret = var.slack_signing_secret
}
```

### Advanced Slack Settings
```hcl
slack {
  bot_token = "xoxb-your-bot-token"
  app_token = "xapp-your-app-token"
  
  admin_channels = ["#admins", "#devops"]
  log_level = "info"
  
  approval {
    timeout = "24h"
    auto_approve = ["@admin1", "@admin2"]
    
    workflow {
      tool_access = true
      file_access = true
      external_api = true
    }
  }
  
  rate_limit {
    messages_per_minute = 60
    tool_calls_per_hour = 100
  }
}
```

## Global Settings

### Logging
```hcl
log_level = "info"  # debug, info, warn, error
log_format = "json" # text, json
log_file = "/var/log/marvin.log"
```

### Security
```hcl
security {
  max_file_size = "100MB"
  allowed_extensions = [".txt", ".md", ".json", ".yaml"]
  scan_uploads = true
  encrypt_config = true
}
```

### Performance
```hcl
performance {
  concurrent_requests = 5
  cache_enabled = true
  cache_ttl = "1h"
  timeout_default = "60s"
}
```

## Environment Variables

### Marvin Configuration
```bash
# Authentication
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_APP_TOKEN=xapp-your-app-token
OPENAI_API_KEY=sk-your-openai-key

# Model settings
MARVIN_MODEL=ministral-3:3b
MARVIN_OLLAMA_HOST=http://localhost:11434

# File locations
MARVIN_CONFIG=/path/to/marvin.hcl
MARVIN_LOG_LEVEL=info
```

#### Configuration Precedence

Marvin determines the configuration file location using the following priority order:

1. **CLI Flag**: `-c` or `--config` flag (highest priority)
2. **Environment Variable**: `MARVIN_CONFIG` environment variable
3. **Default**: `.marvin.hcl` in the current directory (lowest priority)

Example:
```bash
# Environment variable is used when no CLI flag provided
export MARVIN_CONFIG=/etc/marvin/production.hcl
marvin query "test"

# CLI flag overrides environment variable
marvin -c /tmp/debug.hcl query "test"
```

### Docker Environment
```bash
# Container settings
DOCKER_HOST=unix:///var/run/docker.sock
DOCKER_TLS_VERIFY=1
DOCKER_CERT_PATH=/certs

# Resource limits
MARVIN_MEMORY_LIMIT=512MB
MARVIN_CPU_LIMIT=1000m
```

## Command Line Options

### Global Flags
```bash
-c, --config string     Configuration file path
-l, --log-level string   Log level (debug|info|warn|error)
--model string           Override model
--timeout duration       Request timeout
--no-color              Disable colored output
```

### Query Command
```bash
--stream              Enable streaming response
--json                JSON output format
--tool-timeout duration  Tool execution timeout
--max-tokens int      Maximum response tokens
```

### RAG Command
```bash
--chunks int          Number of chunks to retrieve
--similarity float   Similarity threshold (0.0-1.0)
--context-size int    Context window size
--rerank             Enable result reranking
```

## Validation Rules

### Model Validation
- `name` must be valid model identifier
- `host` must be reachable URL
- `timeout` must be valid duration
- `max_tokens` must be positive integer

### Tool Validation
- `executable` must exist on PATH
- `args` must be valid argument list
- MCP servers must respond to health checks

### Security Validation
- No hardcoded secrets in configuration
- File paths must be absolute
- Network URLs must use HTTPS in production
- Certificate validation cannot be disabled

## Configuration Migration

### From v0.x to v1.x
```hcl
# Old format
model "ministral-3:3b" {}

# New format
model "ollama" "main" {
  name = "ministral-3:3b"
}
```

### Environment-Based Migration
```bash
# Check configuration
marvin config validate

# Migrate automatically
marvin config migrate --from-version 0.x
```

## Common Patterns

### Development Configuration
```hcl
model "ollama" "dev" {
  name     = "ministral-3:3b"
  timeout  = "30s"
  log_level = "debug"
}

program "marvin" "dev" {
  model = model.ollama.dev
  filesystem {
    read  = ["./src", "./docs"]
    write = ["./tmp"]
  }
}
```

### Production Configuration
```hcl
model "ollama" "prod" {
  name     = "llama3.1:8b"
  host     = "http://ollama.internal:11434"
  timeout  = "120s"
}

security {
  encrypt_config = true
  scan_uploads = true
  audit_logging = true
}
```

### Testing Configuration
```hcl
model "ollama" "test" {
  name     = "ministral-3:3b"
  timeout  = "10s"
}

program "marvin" "test" {
  model = model.ollama.test
  system_prompt = "You are a test assistant. Respond briefly."
}
```