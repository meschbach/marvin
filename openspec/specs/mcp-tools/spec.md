# MCP Tools Specification

## Overview

Marvin extends LLM capabilities through Model Context Protocol (MCP) tools. MCP tools provide a standardized interface for LLMs to interact with external services, execute commands, and access specialized APIs. This specification documents the currently supported MCP tool transports and their configuration.

### Supported Transports

| Transport | HCL Block | Implementation |
|----------|-----------|----------------|
| Local Programs | `local_program` | `internal/query/localProgram.go` |
| Docker Containers | `docker_mcp` | `internal/query/dockerMCP.go` |
| HTTP Servers | `mcp_over_http` | `internal/query/httpMCP.go` |

### Configuration Discovery

Tools are loaded from the configuration file at the following locations in `internal/config/file.go`:

- **Line 104**: `LocalPrograms []LocalProgramBlock`
- **Line 108**: `DockerMCPBlock []*DockerMCPBlock`
- **Line 109**: `HttpMCPBlock []*HttpMCPBlock`

### Tool Loading Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Tool Loading Flow                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  config.File                                                         │
│      │                                                              │
│      ├── LocalPrograms ──────────────┐                                 │
│      ├── DockerMCPBlock ───────────┼──▶ tooling.Loader.LoadTools    │
│      └── HttpMCPBlock ─────────────┘         │                     │
│                                           ▼                     │
│                                    tooling.Registry              │
│                                           │                     │
│                                           ▼                     │
│                              tooling.Builder.Build                │
│                                           │                     │
│                                           ▼                     │
│                              conversation.ToolSet                │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 1. Local Programs (`local_program` block)

Local programs execute MCP servers as subprocesses on the host system via stdio. This transport is ideal for MCP servers installed locally or in the development environment.

### Configuration

**Config Structure**: `internal/config/localProgramBlock.go:3-9`

| Field | Type | Required | Description |
|-------|------|----------|--------------|
| `name` | `string` | Yes | Unique identifier for the tool instance |
| `program` | `string` | Yes | Path to the MCP server executable |
| `args` | `[]string` | No | Command-line arguments to pass to the executable |
| `sharing` | `*SharingBlock` | No | Access control and sharing configuration |
| `assistant_prompt` | `*AssistantPromptBlock` | No | Custom prompt instructions for the assistant |

### HCL Example

```hcl
local_program "filesystem" {
  program = "/usr/local/bin/mcp-server-filesystem"
  args = ["--root", "/Users/myuser/projects"]
  
  sharing {
    allowed_users = ["user-123", "user-456"]
    allowed_teams = ["engineering"]
    can_share = true
  }
}

local_program "postgres" {
  program = "npx"
  args = ["-y", "@modelcontextprotocol/server-postgres", "--connection-string", "postgresql://localhost:5432/mydb"]
}
```

### Implementation Details

**Runtime**: `internal/query/localProgram.go`

The local program transport uses `mark3labs/mcp-go` stdio transport:

```
Program + Args ──▶ transport.NewStdio() ──▶ MCP Client
```

Key types:
- `localProgramRuntimeSpec` (lines 26-31): Runtime specification holding program and arguments
- `localRunningProgram` (lines 38-47): Active process wrapper

---

## 2. Docker Containers (`docker_mcp` block)

Docker containers run MCP servers as isolated containerized processes. This transport provides better isolation and consistency across environments.

### Configuration

**Config Structure**: `internal/config/dockerMCPBlock.go:9-21`

| Field | Type | Required | Description |
|-------|------|----------|--------------|
| `name` | `string` | Yes | Unique identifier for the tool instance |
| `image` | `string` | Yes | Docker image name (pulled automatically if needed) |
| `args` | `[]DockerMCPBlockArg` | No | Command arguments to pass to container |
| `mount` | `[]DockerMCPMount` | No | Volume mounts (source:target) |
| `env` | `[]DockerMCPBlockEnv` | No | Environment variables |
| `verbose` | `*bool` | No | Enable verbose container output |
| `working_directory` | `string` | No | Working directory inside container |
| `sharing` | `*SharingBlock` | No | Access control and sharing configuration |
| `assistant_prompt` | `*AssistantPromptBlock` | No | Custom prompt instructions |

### Environment Variable Options

**Structure**: `internal/config/dockerMCPBlock.go:38-52`

| Field | Type | Description |
|-------|------|------------|
| `key` | `string` | Environment variable name |
| `value` | `string` | Static value (mutually exclusive with pass_through) |
| `pass_through` | `*bool` | Use host environment variable of the same name |

### Volume Mount Options

**Structure**: `internal/config/dockerMCPBlock.go:58-62`

| Field | Type | Description |
|-------|------|------------|
| `source` | `string` | Host path (resolved relative to config file location) |
| `target` | `string` | Container path |
| `options` | `string` | Docker bind options (e.g., `ro`, `cached`) |

### HCL Example

```hcl
docker_mcp "brave-search" {
  image = "ghcr.io/mark3labs/brave-search-server:latest"
  
  env {
    key = "BRAVE_API_KEY"
    value = "your-api-key-here"
  }
  
  verbose = true
  
  sharing {
    allowed_users = ["user-123"]
    can_share = false
  }
}

docker_mcp "filesystem" {
  image = "alpine:latest"
  args { strings = ["echo", "hello"] }
  
  mount {
    source = "./data"
    target = "/data"
    options = "rw"
  }
  
  working_directory = "/app"
}
```

### Implementation Details

**Runtime**: `internal/query/dockerMCP.go`

Container lifecycle (lines 36-188):
1. **Image Pull**: Inspect image, pull if missing
2. **Container Create**: Configure environment, mounts, arguments
3. **Container Attach**: Connect to container I/O streams
4. **Container Start**: Start the container
5. **MCP Bridge**: Create stdio bridge to container

Container teardown (lines 202-250):
- 15-second timeout for graceful shutdown
- Force remove on conflict
- Automatic cleanup on exit

---

## 3. HTTP Servers (`mcp_over_http` block)

HTTP transports connect to remote MCP servers over HTTP/SSE. This transport is suitable for cloud-hosted MCP servers or services exposed via HTTP.

### Configuration

**Config Structure**: `internal/config/httpMCPBlock.go:3-7`

| Field | Type | Required | Description |
|-------|------|----------|--------------|
| `name` | `string` | Yes | Unique identifier for the tool instance |
| `url` | `string` | Yes | HTTP endpoint URL |
| `sharing` | `*SharingBlock` | No | Access control and sharing configuration |
| `assistant_prompt` | `*AssistantPromptBlock` | No | Custom prompt instructions |

### HCL Example

```hcl
mcp_over_http "remote-search" {
  url = "https://mcp.example.com/search"
  
  sharing {
    allowed_users = ["user-123", "user-456"]
    can_share = true
  }
}

mcp_over_http "github" {
  url = "http://localhost:3000/mcp"
}
```

### Implementation Details

**Runtime**: `internal/query/httpMCP.go`

Uses `mark3labs/mcp-go` streaming HTTP transport:

``` 
URL ──▶ transport.NewStreamableHTTP() ──▶ MCP Client
```

Key types:
- `httpMCPSpec` (lines 25-27): Runtime specification
- `httpMCPEndpoint` (lines 41-52): Active connection wrapper

---

## Tool Registry and Access Policy

### Registry

**File**: `internal/query/tooling/registry.go`

The Registry is a thread-safe container for registered tools indexed by function name:

```
┌─────────────────────────────────────────────┐
│              Registry                        │
├─────────────────────────────────────────────┤
│  map[string]*ToolWithMetadata             │
│                                         │
│  + Get(toolID) → *ToolWithMetadata      │
│  + All() → map[string]*ToolWithMetadata│
│  + Register(ctx, tool, allowedUsers)   │
│  + RegisterToolDef(ctx, tool, name, u)   │
│  + AddUserToTool(toolID, userID)        │
│  + RemoveUserFromTool(toolID, userID)     │
└─────────────────────────────────────────────┘
```

**ToolWithMetadata** (lines 14-17):
```go
type ToolWithMetadata struct {
    Tool         conversation.Tool
    AllowedUsers []string
}
```

### Access Policy

**File**: `internal/query/tooling/access_policy.go`

Controls access to tools based on admin users and per-tool allowed user lists:

```
┌─────────────────────────────────────────────┐
│           AccessPolicy                       │
├─────────────────────────────────────────────┤
│  adminUsers: map[string]bool              │
│                                         │
│  + IsAdmin(userID) bool                 │
│  + CanAccess(ctx, userID, tool) bool    │
└─────────────────────────────────────────────┘
```

**Access Rules**:
1. Admin users bypass all restrictions
2. `nil` allowed users list = open access
3. User in allowed list = access granted
4. Otherwise = access denied

### Tool Loader

**File**: `internal/query/tooling/loader.go`

Loads and validates tools from configuration:

```
Loader.LoadTools(ctx, cfg) → (*Registry, []warnings, error)
```

Process:
1. Iterate each tool configuration block
2. Call `tool.DefineAPI()` to validate and discover functions
3. Extract allowed users from SharingBlock
4. Register each function with allowed users
5. Collect warnings for failures

### Tool Builder

**File**: `internal/query/tooling/builder.go`

Builds user-specific ToolSets filtered by access policy:

```
Builder.Build(ctx, userCtx, registry, policy) → (*ToolSet, error)
```

- Filters registry tools through access policy
- Only includes tools the user is permitted to access
- Returns optional list of denied tools

---

## Tool Sharing Configuration

### SharingBlock

**File**: `internal/config/file.go:247-253`

Controls access and sharing for individual tools:

| Field | Type | Description |
|-------|------|-------------|
| `allowed_users` | `[]string` | User IDs permitted to access this tool |
| `allowed_teams` | `[]string` | Team IDs permitted to access this tool |
| `can_share` | `bool` | Whether users can share this tool with others |
| `expires_at` | `string` | Expiration timestamp (RFC 3339) |
| `auto_approve_shares` | `bool` | Automatic approval for share requests |

### HCL Example

```hcl
local_program "shared-tool" {
  program = "/usr/local/bin/mcp-tool"
  
  sharing {
    allowed_users = ["alice", "bob"]
    allowed_teams = ["engineering", "devops"]
    can_share = true
    auto_approve_shares = false
    expires_at = "2026-12-31T23:59:59Z"
  }
}
```

### Sharing Behavior

- **Empty allowed_users**: Warning logged, no access granted by default
- **No sharing block**: Tool has open access (nil allowed list)
- **With sharing block**: Access restricted to listed users

---

## Tool Loading Integration

### Configuration Loading

```
config.File
    │
    ├── LocalPrograms[] ──▶ Loader.LoadTools ──▶ Registry
    ├── DockerMCPBlock[] ──▶ (auto-discovers functions)
    └── HttpMCPBlock[]  ──▶       │
                              ▼
                        tooling.Registry
                              │
                              ▼
                        Builder.Build(userCtx, registry, policy)
                              │
                              ▼
                        conversation.ToolSet
                              │
                              ▼
                        LLM Tool Calls
```

### Factory Interface

**File**: `internal/query/tooling/loader.go:16-20`

```go
type ToolFactory interface {
    CreateHTTPTool(block *config.HttpMCPBlock) conversation.Tool
    CreateLocalProgramTool(block config.LocalProgramBlock) conversation.Tool
    CreateDockerTool(block *config.DockerMCPBlock) conversation.Tool
}
```

---

## Summary

| Transport | Isolation | Use Case |
|-----------|-----------|----------|
| `local_program` | Process | Local development, installed tools |
| `docker_mcp` | Container | Consistent environments, isolation |
| `mcp_over_http` | Network | Remote/cloud services, existing APIs |

All transports:
- Use `mark3labs/mcp-go` client library
- Support access control via SharingBlock
- Support custom assistant prompts
- Are loaded through the unified Loader/Builder pipeline