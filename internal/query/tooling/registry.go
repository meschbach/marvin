package tooling

import (
	"context"
	"sync"

	"github.com/meschbach/marvin/internal/conversation"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// ToolWithMetadata pairs a conversation.Tool with its access metadata.
// AllowedUsers specifies which users can access this tool; nil means open access.
type ToolWithMetadata struct {
	Tool         conversation.Tool
	AllowedUsers []string
}

// Registry is a thread-safe container for registered tools, indexed by function name.
// Provides concurrent access with read/write locking and tracks per-tool access metadata.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*ToolWithMetadata
}

// NewRegistry creates a new empty Registry ready to accept tool registrations.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]*ToolWithMetadata),
	}
}

// Get retrieves a tool by its function name. Returns the tool metadata and true if found,
// or nil and false if no tool with that name exists.
func (r *Registry) Get(toolID string) (*ToolWithMetadata, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[toolID]
	return tool, ok
}

// All returns a copy of all registered tools indexed by function name.
// The returned map is independent of the internal registry and safe to modify.
func (r *Registry) All() map[string]*ToolWithMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*ToolWithMetadata, len(r.tools))
	for k, v := range r.tools {
		result[k] = v
	}
	return result
}

// Register adds a tool to the registry, inferring function names from its API definition.
// The tool's DefineAPI is called to discover available functions, and each is registered
// with the provided allowed users list. Silently skips registration if DefineAPI fails.
func (r *Registry) Register(ctx context.Context, tool conversation.Tool, allowedUsers []string) {
	ctx, span := tracer.Start(ctx, "Registry.Register", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	definition, err := tool.DefineAPI(ctx)
	if err != nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range definition.Tool {
		r.tools[definition.Tool[i].Function.Name] = &ToolWithMetadata{
			Tool:         tool,
			AllowedUsers: allowedUsers,
		}
	}
}

// RegisterToolDef registers a tool under a specific function name, bypassing API discovery.
// Useful when the tool's function name is known upfront or when registering individual functions.
func (r *Registry) RegisterToolDef(ctx context.Context, tool conversation.Tool, toolName string, allowedUsers []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools[toolName] = &ToolWithMetadata{
		Tool:         tool,
		AllowedUsers: allowedUsers,
	}
}

var tracer = otel.Tracer("github.com/meschbach/marvin/internal/query/tooling")
