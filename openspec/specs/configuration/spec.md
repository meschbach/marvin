# Configuration Specification

## Overview

Marvin uses HCL (HashiCorp Configuration Language) for declarative configuration of the AI tooling assistant. The configuration system supports both CLI and Slacker deployments with a unified configuration format.

### Configuration File Locations

| Priority | Source | Description |
|----------|--------|-------------|
| 1 | `-c` flag | CLI-specified config path |
| 2 | `MARVIN_CONFIG` env | Environment variable |
| 3 | `.marvin.hcl` | Default config file |
| 4 | `marvin.test.hcl` | Test config file |

### Default Values

| Setting | Default |
|---------|---------|
| Language Model | `ministral-3:3b` |
| Embedding Model | `mxbai-embed-large:latest` |
| Provider | `ollama` |
| Show Thinking | `false` |
| Show Tools | `true` |
| Show Done | `true` |
| Verbose | `false` |

---

## Configuration File Structure

The `File` struct at `internal/config/file.go:93-116` represents the root configuration object. All HCL blocks are children of this root.

```go
type File struct {
    Model           string                // Model name
    ProviderName   string                // Provider type (ollama, openrouter, gemini)
    OpenRouter     *OpenRouterBlock     // OpenRouter configuration
    Gemini         *GeminiBlock          // Google Gemini configuration
    Options        *ModelOptionsBlock    // Model inference options
    LocalPrograms  []LocalProgramBlock   // Local MCP tool servers
    SystemPrompt   *SystemPromptBlock   // System prompt configuration
    Documents      []*DocumentsBlock    // RAG document configurations
    DockerMCPBlock []*DockerMCPBlock   // Docker-based MCP tools
    HttpMCPBlock   []*HttpMCPBlock     // HTTP-based MCP tools
    MultiTenant    *MultiTenantBlock    // Multi-tenant (Slacker) configuration
    ModelAccess    *ModelAccessBlock   // Model access control
    Display       *DisplayBlock        // Output display preferences
    Observability *ObservabilityBlock // OTEL tracing configuration
}
```

---

## Block Types

### 1. Model Configuration Block

**Definition:** `internal/config/file.go:96-98`

The model block specifies the LLM to use for conversations.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model` | string | No | LLM model name (e.g., `llama3.2:latest`, `ministral-3:3b`) |
| `provider` | string | No | Provider type: `ollama`, `openrouter`, or `gemini` |

**Example:**
```hcl
model = "llama3.2:latest"
provider = "ollama"
```

---

### 2. Model Options Block

**Definition:** `internal/config/file.go:55-75`

Advanced options for fine-tuning model inference.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `context_window_size` | int | `2048` | Context window size in tokens |
| `temperature` | float32 | `0.8` | Sampling temperature (0.0-1.0) |
| `top_p` | float32 | `0.9` | Nucleus sampling parameter |
| `top_k` | int | `40` | Top-k sampling (-1 = no limit) |
| `num_predict` | int | `-1` | Max tokens to predict |
| `repeat_penalty` | float32 | `1.1` | Repetition penalty |
| `repeat_last_n` | int | `64` | Lookback for repetitions |
| `seed` | int | - | Random seed for reproducibility |
| `stop` | []string | - | Stop sequences |

**Example:**
```hcl
options {
    context_window_size = 4096
    temperature = 0.7
    num_predict = 2048
    stop = ["###", "END"]
}
```

---

### 3. Provider Blocks

#### 3a. Ollama (Default)

Ollama requires no explicit configuration block. The provider defaults to `ollama` at `internal/config/file.go:140-144`.

#### 3b. OpenRouter Block

**Definition:** `internal/config/file.go:40-53`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `api_key` | string | No* | OpenRouter API key |
| `api_key_file` | string | No* | Path to file containing API key |
| `base_url` | string | No | Custom endpoint URL |

*One of `api_key` or `api_key_file` required if using OpenRouter provider.

**Environment Priority:** `OPENROUTER_API_KEY` > config file

**Example:**
```hcl
provider = "openrouter"
model = "qwen2.5:7b"

openrouter {
    api_key_file = "/path/to/openrouter-api-key.txt"
    base_url = "https://openrouter.ai/api/v1"
}
```

#### 3c. Gemini Block

**Definition:** `internal/config/file.go:27-38`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `api_key` | string | No* | Gemini API key |
| `api_key_file` | string | No* | Path to file containing API key |

*One of `api_key` or `api_key_file` required if using Gemini provider.

**Environment Priority:** `GEMINI_API_KEY` > config file

**Example:**
```hcl
provider = "gemini"
model = "gemini-2.0-flash"

gemini {
    api_key_file = "/path/to/gemini-api-key.txt"
}
```

---

### 4. System Prompt Block

**Definition:** `internal/config/file.go:161-164`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from_string` | string | No* | Inline prompt text |
| `from_file` | string | No* | Path to prompt file |

*One of `from_string` or `from_file` required.

**Example:**
```hcl
system_prompt {
    from_string = <<EOS
You are a helpful AI assistant integrated with Slack.
EOS
}

# Or from file
system_prompt {
    from_file = "./prompts/system.txt"
}
```

---

### 5. MCP Tool Blocks

#### 5a. Local Program Block

**Definition:** `internal/config/localProgramBlock.go:3-9`

| Field | Type | Label | Description |
|-------|------|-------|-------------|
| `name` | string | Yes | Tool identifier |
| `program` | string | Yes | Executable path |
| `args` | []string | No | Command arguments |
| `sharing` | block | No | Sharing restrictions |
| `assistant_prompt` | block | No | Tool description for AI |

**Example:**
```hcl
local_program "gitea" {
    program = "/usr/local/bin/gitea-mcp-server"
    args = ["-host", "https://gitea.example.com"]
    
    assistant_prompt {
        from_string = "You have access to Gitea repositories..."
    }
}
```

#### 5b. Docker MCP Block

**Definition:** `internal/config/dockerMCPBlock.go:9-21`

| Field | Type | Label | Description |
|-------|------|-------|-------------|
| `name` | string | Yes | Tool identifier |
| `image` | string | Yes | Docker image |
| `args` | block | No | Container arguments |
| `mount` | block | No | Volume mounts |
| `env` | block | No | Environment variables |
| `verbose` | bool | No | Enable verbose logging |
| `working_directory` | string | No | Container working directory |
| `sharing` | block | No | Sharing restrictions |
| `assistant_prompt` | block | No | Tool description for AI |

**Example:**
```hcl
docker_mcp "postgres" "postgres:15" {
    env "POSTGRES_PASSWORD" {
        pass_through = true
    }
    
    mount "data" "/data" {
        options = "ro"
    }
    
    assistant_prompt {
        from_string = "You have access to a PostgreSQL database..."
    }
}
```

#### 5c. HTTP MCP Block

**Definition:** `internal/config/httpMCPBlock.go`

| Field | Type | Label | Description |
|-------|------|-------|-------------|
| `name` | string | Yes | Tool identifier |
| URL | string | Yes | MCP server endpoint |
| `assistant_prompt` | block | No | Tool description for AI |

**Example:**
```hcl
mcp_over_http "weather-api" "https://weather.example.com/mcp" {
    assistant_prompt {
        from_string = "You have access to weather data..."
    }
}
```

---

### 6. Multi-Tenant Block

**Definition:** `internal/config/multitenant.go:76-84`

Configuration for multi-tenant Slacker deployments.

| Field | Type | Description |
|-------|------|-------------|
| `admin_users` | []string | Admin user IDs (bypass all restrictions) |
| `admin_channel` | string | Admin notification channel |
| `session_store_path` | string | User session storage directory |
| `credential_store` | string | User credential storage directory |
| `slacker_state_path` | string | Slacker-specific state directory |
| `security_log_format` | string | Custom security log format |
| `approval_timeout` | string | Tool approval timeout (e.g., `24h`) |
| `cron` | block | Scheduled message jobs |

**Example:**
```hcl
multi_tenant {
    admin_users = ["U123456789", "U987654321"]
    admin_channel = "#admin-alerts"
    session_store_path = "./sessions"
    credential_store = "./credentials"
    slacker_state_path = "./slacker-state"
    approval_timeout = "24h"
}
```

#### Cron Stanza

**Definition:** `internal/config/cron.go:4-13`

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Job identifier |
| `schedule` | string | Cron expression |
| `send_to` | string | Target user/channel |
| `message` | string | Message content |

**Example:**
```hcl
cron "daily_summary" {
    schedule = "0 9 * * *"
    send_to = "#general"
    message = "Daily summary: ..."
}
```

---

### 7. Model Access Block

**Definition:** `internal/config/file.go:232-236`

Access control for models in Slacker operations.

| Field | Type | Description |
|-------|------|-------------|
| `allowed_models` | []string | Explicitly permitted models |
| `denied_models` | []string | Explicitly blocked models |

**State File:** `slacker-state/model-access.json`

The state file overrides HCL configuration and is managed programmatically.

**Example:**
```hcl
model_access {
    allowed_models = ["llama3.2:latest", "qwen2.5:7b"]
    denied_models = ["experimental:beta"]
}
```

---

### 8. Display Block

**Definition:** `internal/config/file.go:77-91`

Output formatting preferences.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `show_thinking` | bool | `false` | Show AI thinking process |
| `show_tools` | bool | `true` | Show tool invocations |
| `show_done` | bool | `true` | Show completion messages |
| `verbose` | bool | `false` | Enable debug output |
| `thinking_format` | string | `plain` | Thinking format: `plain`, `markdown`, `collapsed` |
| `tool_format` | string | `detailed` | Tool format: `simple`, `detailed`, `json` |

**Resolution Order:** CLI flags > user session > HCL > defaults

**Example:**
```hcl
display {
    show_thinking = true
    thinking_format = "markdown"
    tool_format = "detailed"
}
```

---

### 9. Observability Block

**Definition:** `internal/config/file.go:171-183`

OpenTelemetry tracing configuration.

| Field | Type | Description |
|-------|------|-------------|
| `exporter` | string | OTEL exporter: `none`, `stdout`, `grpc` |
| `service_name` | string | Service name for tracing |
| `environment` | string | Deployment environment |
| `batched` | bool | Enable batched span export (default: true) |
| `silent` | bool | Suppress startup output |

**Environment Overrides:** `OTEL_EXPORTER`, `OTEL_SERVICE_NAME`, `ENV`

**Example:**
```hcl
observability {
    exporter = "grpc"
    service_name = "marvin"
    environment = "production"
}
```

---

### 10. Documents Block

**Definition:** `internal/config/documentsBlock.go:15-22`

RAG document configuration for semantic search.

| Field | Type | Label | Description |
|-------|------|-------|-------------|
| `name` | string | Yes | Collection name |
| `document_path` | string | Yes | Path to index |
| `storage_path` | string | Yes | Vector DB storage path |
| `description` | string | No | Collection description |
| `model` | string | No | Embedding model |

**Example:**
```hcl
documents "docs" "./docs" {
    storage_path = "./vector-db/docs"
    description = "Internal documentation"
}
```

---

## Configuration Loading

### Load Process

**Definition:** `internal/config/config.go:36-67`

```
1. Get config path (CLI flag → MARVIN_CONFIG → .marvin.hcl)
2. Parse HCL file with hclparse
3. Decode into File struct with gohcl.DecodeBody
4. Resolve working directory (config file directory)
5. Apply CLI overrides (OpenRouter key file)
6. Return configured File
```

### API Reference

#### `resolveWorkingDirectory`

**Definition:** `internal/config/file.go:118-128`

Resolves the working directory to the configuration file's directory and propagates it to Docker MCP blocks.

```go
func (f *File) resolveWorkingDirectory(marvinFilePath string) (string, error)
```

#### `LanguageModel`

**Definition:** `internal/config/file.go:130-137`

Returns the configured model name or the default.

```go
func (f *File) LanguageModel() string
```

#### `Provider`

**Definition:** `internal/config/file.go:139-145`

Returns the provider type, defaulting to Ollama.

```go
func (f *File) Provider() ProviderType
```

#### `BuildAPIOptions`

**Definition:** `internal/config/file.go:260-310`

Constructs API request options, including only explicitly set values.

```go
func (f *File) BuildAPIOptions() map[string]any
```

#### `ValidateModelAccess`

**Definition:** `internal/config/file.go:421-477`

Validates model access for Slacker operations.

```go
func (f *File) ValidateModelAccess(model string, userID string) (bool, string)
```

---

## Example Files

### Basic CLI Configuration

See: `marvin.example.hcl`

```hcl
model = "llama3.2:latest"

options {
    context_window_size = 4096
    temperature = 0.7
}

system_prompt {
    from_string = "You are a helpful AI assistant..."
}

display {
    show_thinking = true
    thinking_format = "markdown"
}

local_program "docs" {
    program = "file-mcp"
}

docker_mcp "postgres" "postgres:15" {
    assistant_prompt {
        from_string = "You have access to PostgreSQL..."
    }
}

mcp_over_http "weather" "https://weather-api:8080" {
    assistant_prompt {
        from_string = "You have access to weather data..."
    }
}
```

### Slacker Configuration

See: `marvin.slacker.example.hcl`

```hcl
model = "ministral-3:3b"

options {
    context_window_size = 32768
    temperature = 0.7
}

multi_tenant {
    admin_users = ["U1234567890"]
    admin_channel = "#admin"
    session_store_path = "./sessions"
    credential_store = "./credentials"
}

model_access {
    allowed_models = ["llama3.2:latest", "qwen2.5:7b"]
    denied_models = ["experimental:beta"]
}

mcp_over_http "weather-api" "https://weather.example.com/mcp" {
    assistant_prompt {
        from_string = "You have access to weather data..."
    }
}
```

---

## Related Specifications

- [MCP Tools Specification](../mcp-tools/spec.md)
- [Slacker Specification](../slacker/spec.md)