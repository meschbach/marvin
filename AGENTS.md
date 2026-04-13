# AGENTS.md

This file provides guidelines for agentic coding agents working in this repository.

## Project Overview

Marvin is a Go-based agentic workflow CLI that connects AI reasoning loops to Model Context Protocol (MCP) tools. It's built with:
- **Language**: Go 1.26+
- **CLI Framework**: Cobra
- **Configuration**: HCL (HashiCorp Configuration Language)
- **LLM Integration**: Ollama API
- **Tool Protocol**: MCP (Model Context Protocol)

## Build and Test Commands

### Core Commands
```bash
# Build the main binary
go build -o marvin ./cmd/marvin

# Run from source (development)
go run ./cmd/marvin query "your query here"

# Run all unit tests
go test -count 1 ./internal/...

# Run a specific test
go test -run TestLoadConfig_EmptyFile ./internal/config/
go test -run TestLoadConfig_SingleLocalProgram ./internal/config/
go test -run TestLoadConfig_AllOptionsMultipleBlocks ./internal/config/

# Full integration test suite
./test.sh
```

### Build for Release
```bash
# Build release artifacts for all platforms
./release.sh

# Manual cross-compilation example
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-w -s -extldflags "-static"' -o marvin ./cmd/marvin
```

### Testing in CI
The project uses GitHub Actions with:
- Unit tests: `go test -count 1 ./internal/...`
- Integration tests: `./test.sh` (requires Ollama with specific models)

### Pre-commit Hooks
The project uses pre-commit hooks to ensure code quality standards and provide fast developer feedback:

**Setup:**
```bash
# Install pre-commit
pip install pre-commit

# Install hooks for this repository
pre-commit install

# Run hooks on all files (initial setup or verification)
pre-commit run --all-files

# Update hooks to latest versions
pre-commit autoupdate
```

**Hook Configuration:**
- `go fmt` - Auto-format Go code (auto-fixes)
- `go mod tidy` - Clean up dependencies (auto-fixes)
- `go vet` - Static analysis for common issues
- `golangci-lint` - Comprehensive linting with default settings
- `go test -count 1 ./internal/...` - Unit tests (fast, matches CI)
- `go build` - Verify marvin and slacker binaries compile

**Development Workflow:**
- Hooks run automatically on `git commit`
- Failed commits will show specific error messages
- Auto-fix available for formatting and dependency issues
- Skip if needed: `git commit --no-verify` (use sparingly)
- Target execution time: < 30 seconds total

**Agent Usage:**
When implementing changes, ensure all pre-commit hooks pass before considering work complete:
```bash
# Verify manually before finishing
pre-commit run --all-files

# Run specific hook category
pre-commit run go-fmt go-mod-tidy go-vet
pre-commit run golangci-lint
pre-commit run go-test-unit go-build-marvin go-build-slacker
```

**Final Verification Checklist (major milestones or completion):**
Run these commands to verify the entire project passes linting - do NOT lint individual files (causes typecheck errors):
```bash
# Lint on packages (not files)
golangci-lint run ./...

# Full unit tests
go test -count 1 ./internal/...

# Verify all packages compile
go build ./...
```

## Code Style Guidelines

### File and Package Structure
- **Entry Point**: `cmd/main.go` - Cobra command setup and routing
- **Core Logic**: `internal/query/` - Query handling, tool integration, LLM interaction
- **Configuration**: `internal/config/` - HCL parsing and configuration management
- **Commands**: `cmd/*.go` - Individual command implementations (query, rag, mcp, etc.)

### Import Conventions
```go
// Standard library first
import (
    "context"
    "fmt"
    "os"
)

// External dependencies second
import (
    "github.com/hashicorp/hcl/v2"
    "github.com/spf13/cobra"
    "github.com/ollama/ollama/api"
)

// Internal packages last
import (
    "github.com/meschbach/marvin/internal/config"
)
```

### Naming Conventions
- **Files**: `snake_case.go` (e.g., `parse_test.go`, `docker_mcp.go`)
- **Packages**: `lowercase` (e.g., `config`, `query`)
- **Functions**: `CamelCase` with exported names public, `camelCase` for private
- **Variables**: `camelCase` for local variables, `CamelCase` for exported constants
- **Types**: `CamelCase` for structs and interfaces

### Documentation Guidelines
- **All code elements must have documentation** - Every exported function, type, and constant
- **Focus on WHAT and WHY** - Describe the purpose and rationale, not implementation details
- **Concise and clear** - Avoid verbose explanations; be direct
- **No implementation details** - Don't explain HOW the code works, only WHAT it does and WHY it exists
- **Document the contract** - Explain behavior, inputs, outputs, and side effects

**Examples:**
```go
// ConversationStats tracks token usage metrics for a single conversation.
// Provides real-time visibility into LLM resource consumption.
type ConversationStats struct {
    PromptTokens   int
    ResponseTokens int
    TotalTokens    int
}

// RunConversation executes the conversation loop until completion or error.
// Handles tool calls, streaming responses, and session persistence.
func (e *ConversationEngine) RunConversation(ctx context.Context, model string, updater StreamingUpdater) error
```

**Anti-patterns to avoid:**
```go
// This function loops through messages and calls the chat API.  // ❌ Explains HOW, not WHAT
// It processes the response in a for loop.
```

### Error Handling Pattern
```go
// Standard error handling with context
client, err := api.ClientFromEnvironment()
if err != nil {
    fmt.Fprintf(os.Stderr, "Error creating Ollama client: %v\n", err)
    return
}

// Use require/assert in tests
cfg, err := interpretConfigFile(parseHCLString(t, hcl, "test.hcl"), "/test")
require.NoError(t, err)
assert.NotNil(t, cfg)
```

### Configuration Management
- **Format**: HCL configuration files
- **Default Config**: `.marvin.hcl` (configurable via `-c` flag)
- **Test Config**: `marvin.test.hcl`
- **Example Config**: `marvin.example.hcl`

### Testing Patterns
- Use table-driven tests for multiple scenarios
- Test helpers should use `t.Helper()`
- Use `testify/assert` and `testify/require` for assertions
- Include both positive and negative test cases
- Test configuration parsing with HCL strings

### Tool Integration
- MCP tools are configured via HCL blocks
- Tools are initialized in `internal/query/` package
- Each tool type has its own implementation file (e.g., `dockerMCP.go`, `httpMCP.go`)
- Tools must implement proper shutdown/cleanup patterns

### LLM Integration
- Primary LLM: Ollama (default model: `ministral-3:3b`)
- Streaming responses are handled via context patterns
- Tool calls are intercepted and displayed
- System prompts are configurable via HCL

### Slack Integration
- **Uses Socket Mode API** (modern, replaces deprecated RTM)
- Multi-tenant bot with approval workflows
- Tool management via natural language and slash commands
- Session management and security logging
- Interactive components and rich event handling
- Environment variables: `SLACK_BOT_TOKEN` (xoxb-...), `SLACK_APP_TOKEN` (xapp-...)

### Model Access Control
- **Scope Separation**: Model access control applies to Slack/Slacker operations only
- **CLI Bypass**: CLI operations use whatever models are in their configuration (no validation)
- **Admin System**: Uses existing `AdminUsers` from `MultiTenantBlock` (no separate model admins)
- **State Management**: Dedicated `slacker_state_path` directory with JSON state files
- **Security Logging**: Integration with existing `SecurityLogFormat` for access violations
- **Configuration Priority**: State files override HCL configuration for Slacker operations
- **Default Model**: Always permitted as system fallback (`ministral-3:3b`)

#### State Management Patterns
- **Dedicated Directory**: `slacker_state_path` in multi-tenant configuration
- **JSON Format**: `slacker-state/model-access.json` for programmatic access
- **Atomic Writes**: Use temp-file + rename pattern for data integrity
- **Admin Commands**: Natural language commands for model access management

#### Integration Points
- **Validation Location**: `internal/slacker/query_streaming.go:88` - Add model validation before LLM calls
- **CLI Integration**: `cmd/list_models.go` - Model discovery with access status awareness
- **Configuration**: `internal/config/file.go` - Core validation logic and state management
- **Admin Interface**: `internal/slacker/message_handler.go` - Intent processing and response handling

#### Configuration Examples
```hcl
multi_tenant {
    admin_users = ["admin-user-id"]
    session_store_path = "./sessions"
    credential_store = "./credentials"
    slacker_state_path = "./slacker-state"  # Dedicated state directory
}

model_access {
    allowed_models = ["llama3.2:latest", "qwen2.5:7b"]
    denied_models = ["experimental:beta"]
}
```

#### CLI vs. Slacker Behavior
- **CLI Operations**: No model validation - bypass all access restrictions
- **Slacker Operations**: Full model validation with allow/deny list enforcement
- **Admin Users**: Bypass all restrictions in Slack (but not in CLI by design)
- **Security Events**: Logged for denied access and fallback operations

### CLI Patterns
- All commands use Cobra framework
- Global options are handled through `config.CommandLineOptions`
- Commands should be composable and testable
- Use `PersistentFlags()` for global configuration options

### Code Organization Principles
- Keep `internal/` for project-private code
- External integrations in their own files within appropriate packages
- Configuration structs should match HCL block structure
- Use interfaces for tool implementations to enable testing

### Component Design Principles
- **Prefer small components**: Many small, focused components are better than few large ones
- **Consumer-defined interfaces**: Interfaces defined by components that need them, not implementers
- **Avoid unnecessary abstraction**: Don't create interfaces for single implementations
- **Direct dependencies**: Use concrete types when there's only one implementation
- **Component cohesion**: Keep related functionality together in focused files

### File Size and Refactoring Guidelines
- **Target file sizes**: <200 lines (ideal), <400 lines (maximum)
- **Keep structs and methods together**: Maintain Go conventions for readability
- **Functional grouping**: Group related functionality together in focused files
- **Single responsibility**: Each file should have one primary purpose
- **No types files**: Avoid separating struct definitions from their methods
- **No centralized interfaces**: Define interfaces inline where they're used

### Refactoring Strategy
When files exceed size targets:
1. **Analyze responsibilities**: Identify distinct functional areas within the file
2. **Split into focused components**: Create smaller structs with single responsibilities
3. **Maintain cohesion**: Keep struct definitions with their core methods in same files
4. **Consumer-defined interfaces**: Define interfaces where needed by consuming components
5. **Avoid over-abstraction**: Use concrete types unless testing or multiple implementations
6. **Preserve functionality**: Ensure all existing behavior is maintained
7. **Test thoroughly**: Verify refactoring doesn't break existing functionality

### Interface Guidelines
- **Define by consumers**: Interfaces defined by components that need the behavior
- **Single implementations**: Avoid interfaces with only one implementation
- **Testing focus**: Use interfaces primarily for testability
- **Inline definitions**: Define interfaces in same file as consumer, not centralized

### Dependencies and External Libraries
- **Cobra**: CLI framework
- **HCL v2**: Configuration parsing
- **Ollama API**: LLM integration
- **Docker SDK**: Container tool integration
- **Chromem-go**: Vector storage for RAG
- **Testify**: Testing utilities

### Performance Considerations
- Use streaming for LLM responses
- Implement proper context cancellation
- Tool cleanup should be handled in defer blocks
- Configuration loading is done once per command execution

### Security Notes
- Never log or expose sensitive configuration data
- Validate all external program inputs
- Use proper error channels for user feedback
- Tokens and credentials should be handled via environment variables or secure config

### Observability and Tracing
- **Span Naming Convention**: Use `StructName.MethodName` format (e.g., `MessageHandler.ProcessMessage`, `QueryStreamer.ProcessQueryWithUpdater`)
- **Context Propagation**: Always pass the incoming context to child spans; never use `context.Background()` in event handlers as it breaks the trace chain
- **Key Integration Points**:
  - `internal/slacker/events.go` - Slack event entry point creates root span
  - `internal/slacker/message_handler.go` - Message processing and routing
  - `internal/slacker/query_handler.go` - Query processing (ensure ctx propagation to async operations)
  - `internal/slacker/query_streaming.go` - LLM integration and streaming
- **Attributes**: Add relevant attributes like user ID, channel ID, intent action, and confidence scores to aid debugging

### Documentation Guidelines
- **Update both README.md and docs/** - When adding features or making significant changes, update documentation in both locations
- **README.md** - High-level overview, quick start, and usage examples
- **docs/** - Comprehensive documentation organized by topic:
  - `docs/configuration/` - Detailed configuration reference
  - `docs/deployment/` - Production deployment guides
  - `docs/development/` - Development and testing documentation
  - `docs/integrations/` - Integration guides with other systems
  - `docs/slacker/` - Slack-specific documentation
- **Keep documentation in sync** - Ensure consistency between README.md and docs/ content
- **Use appropriate specificity** - README.md for quick reference, docs/ for detailed explanations