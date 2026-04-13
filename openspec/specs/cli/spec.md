# Marvin CLI Specification

## Overview

Marvin is a Go-based agentic workflow CLI that connects AI reasoning loops to Model Context Protocol (MCP) tools. It provides direct terminal access to LLM capabilities with tool integration.

**Entry Point**: `cmd/marvin/main.go`

## Commands

| Command | Description | File:Line |
|---------|-------------|-----------|
| `query` | Send free-form queries to Ollama | `cmd/marvin/llm.go:13-48` |
| `goal` | Declare high-level goals for session | `cmd/marvin/llm.go:50-69` | 
| `rag` | RAG operations (index, query) | `cmd/marvin/rag.go:12-65` |
| `mcp list` | List available MCP tools | `cmd/marvin/mcp.go:13-36` |
| `list-models` | List available Ollama models | `cmd/marvin/list_models.go:15-134` |

---

## Query Command

**Usage**: `marvin query "your query here"`

Sends a free-form query to Ollama and prints the response. Supports various display options for debugging and visualization.

**Flags**:

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--verbose` | `-v` | false | Show debugging statements |
| `--show-thinking` | `-t` | false | Show the model's thinking process |
| `--show-tools` | `-s` | false | Show tools available and usage |
| `--dump-tools` | `-d` | false | Dump the available tools to the LLM |
| `--show-done` | `-e` | false | Show the Done command issued by the LLM |
| `--thinking-format` | | empty | Thinking display format (plain, markdown, collapsed) |

**Examples**:
```bash
marvin query "What files were modified recently?"
marvin query --show-thinking "Explain how the cache works"
marvin query -v -d "debug the auth issue"
```

---

## Goal Command

**Usage**: `marvin goal "my goal"`

Declares a high-level goal for the current session. This is useful for providing context to subsequent queries.

**File**: `cmd/marvin/llm.go:50-69`

**Examples**:
```bash
marvin goal "Fix the authentication bug in the login flow"
marvin goal "Review and refactor the database layer"
```

---

## RAG Command

**Usage**: `marvin rag <subcommand>`

Operations against the RAG (Retrieval-Augmented Generation) store for document indexing and querying.

**File**: `cmd/marvin/rag.go:12-65`

### Subcommands

#### rag index

**Usage**: `marvin rag index`

Indexes all documents from the configuration file. Uses SIGTERM/SIGINT signal handling for graceful interruption (SIGSTOP is not trap-able).

#### rag query

**Usage**: `marvin rag query <store> <query>`

Queries the RAG store with a specific store name and query string.

**Arguments**:
- `<store>` - The RAG store name to query
- `<query>` - The search query string

**Examples**:
```bash
marvin rag index
marvin rag query docs "how do I configure authentication"
```

---

## MCP List Command

**Usage**: `marvin mcp list [flags]`

Lists available MCP (Model Context Protocol) tools from the configured MCP servers.

**File**: `cmd/marvin/mcp.go:13-36`

**Flags**:

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--detailed` | `-d` | false | Provides detailed output for each tool |

**Examples**:
```bash
marvin mcp list
marvin mcp list --detailed
```

---

## List Models Command

**Usage**: `marvin list-models [flags]`

Lists available Ollama models and optionally shows their access status for Slacker operations.

**File**: `cmd/marvin/list_models.go:15-134`

**Flags**:

| Flag | Default | Description |
|------|---------|-------------|
| `--access` | false | Show model access status for Slacker operations |

**Examples**:
```bash
marvin list-models
marvin list-models --access
```

When `--access` is provided, output includes:
- Model name and size
- Status (Available/Denied)
- Access status (Allowed/Denied with reason)
- Default model indicator

---

## Global Configuration

### Config File Loading

**Files**:
- `cmd/marvin/main.go:15-18` - globalOptions struct
- `internal/config/config.go:36-49` - Load() function

### Default Config File

**Location**: `.marvin.hcl` in the current working directory

### Configuration Methods (Priority Order)

1. **CLI Flag**: `-c` or `--config` flag
2. **Environment Variable**: `MARVIN_CONFIG`
3. **Default**: `.marvin.hcl`

### Global Flags

| Flag | Description |
|------|-------------|
| `-c, --config` | Path to the configuration file |
| `--openrouter-key-file` | Path to a file containing the OpenRouter API key |

### Display Preferences

Display preferences can be configured in the HCL config file under the `display` block, or overridden via CLI flags on the `query` command.

**Config File Structure** (`internal/config/file.go:77-91`):

```hcl
display {
  show_thinking   = true
  show_tools      = false
  show_done       = false
  verbose         = false
  thinking_format = "plain"  # "plain", "markdown", or "collapsed"
  tool_format     = "simple" # "simple", "detailed", or "json"
}
```

**Config Options**:

| Option | Type | Description |
|--------|------|-------------|
| `show_thinking` | bool | Show model thinking process |
| `show_tools` | bool | Show tool invocation details |
| `show_done` | bool | Show completion messages |
| `verbose` | bool | Show verbose debugging output |
| `thinking_format` | string | Thinking display format (plain, markdown, collapsed) |
| `tool_format` | string | Tool display format (simple, detailed, json) |

**CLI Override Mapping**:

| CLI Flag | Config Option | Type |
|----------|---------------|------|
| `--verbose` | `verbose` | bool |
| `--show-thinking` | `show_thinking` | bool |
| `--show-tools` | `show_tools` | bool |
| `--show-done` | `show_done` | bool |
| `--thinking-format` | `thinking_format` | string |

---

## Configuration File Example

```hcl
# Model configuration
model = "ministral-3:3b"
provider = "ollama"

# Optional: OpenRouter configuration
# openrouter {
#   api_key = "your-key-here"
# }

# Optional: Google Gemini configuration
# gemini {
#   api_key = "your-key-here"
# }

# Model options
options {
  temperature     = 0.7
  top_p           = 0.9
  num_predict     = 2048
}

# Display preferences
display {
  show_thinking   = false
  show_tools      = false
  show_done       = false
  verbose         = false
  thinking_format = "plain"
}

# Local programs available as tools
local_program "git" {
  program = "/usr/bin/git"
}

# RAG documents for context
documents {
  repository = "./docs"
}

# MCP servers
docker_mcp {
  name = "filesystem"
  image = "ghcr.io/stateful/mcp-filesystem:latest"
  args = ["/data"]
}
```

---

## Notes

- All commands use the Cobra CLI framework
- Configuration loading is done once per command execution
- The CLI does not enforce model access restrictions (unlike Slacker operations)
- RAG operations support graceful shutdown via SIGTERM/SIGINT signal (SIGSTOP is not trap-able)