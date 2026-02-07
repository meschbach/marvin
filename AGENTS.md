# AGENTS.md

This file provides guidelines for agentic coding agents working in this repository.

## Project Overview

Marvin is a Go-based agentic workflow CLI that connects AI reasoning loops to Model Context Protocol (MCP) tools. It's built with:
- **Language**: Go 1.25+
- **CLI Framework**: Cobra
- **Configuration**: HCL (HashiCorp Configuration Language)
- **LLM Integration**: Ollama API
- **Tool Protocol**: MCP (Model Context Protocol)

## Build and Test Commands

### Core Commands
```bash
# Build the main binary
go build -o marvin ./cmd

# Run from source (development)
go run ./cmd query "your query here"

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
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-w -s -extldflags "-static"' -o marvin ./cmd
```

### Testing in CI
The project uses GitHub Actions with:
- Unit tests: `go test -count 1 ./internal/...`
- Integration tests: `./test.sh` (requires Ollama with specific models)

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