package tooling

import (
	"context"

	"github.com/meschbach/marvin/internal/conversation"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// UserInfo holds contextual information about the user making a tool request.
type UserInfo struct {
	UserID string
}

// Builder constructs ToolSets for users based on access policies.
// Filters available tools from a Registry to only include those the user is permitted to use.
type Builder struct{}

// NewBuilder creates a new Builder instance for constructing user-specific tool sets.
func NewBuilder() *Builder {
	return &Builder{}
}

// Build constructs a ToolSet for the given user by filtering tools from the registry
// through the access policy. Only tools the user is permitted to access are included.
// Returns the filtered ToolSet or an error if tool definitions cannot be loaded.
func (b *Builder) Build(ctx context.Context, userCtx *UserInfo, registry *Registry, policy *AccessPolicy) (*conversation.ToolSet, error) {
	ctx, span := tracer.Start(ctx, "ToolBuilder.Build", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	toolSet, _, err := b.BuildWithDeniedInfo(ctx, userCtx, registry, policy)
	return toolSet, err
}

// BuildWithDeniedInfo constructs a ToolSet for the given user, identical to Build,
// but also returns a list of tool names that were denied access due to the policy.
// This is useful for informing users which tools they do not have access to.
func (b *Builder) BuildWithDeniedInfo(ctx context.Context, userCtx *UserInfo, registry *Registry, policy *AccessPolicy) (*conversation.ToolSet, []string, error) {
	ctx, span := tracer.Start(ctx, "ToolBuilder.BuildWithDeniedInfo", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	toolSet := conversation.NewToolSet()
	var deniedTools []string
	allTools := registry.All()

	for name, toolMeta := range allTools {
		if !policy.CanAccess(ctx, userCtx.UserID, toolMeta) {
			deniedTools = append(deniedTools, name)
			continue
		}

		def, err := toolMeta.Tool.DefineAPI(ctx)
		if err != nil {
			continue
		}

		b.addToolDefs(toolSet, toolMeta.Tool, def)
	}

	span.SetAttributes(
		attribute.String("user.id", userCtx.UserID),
		attribute.Int("tools.count", len(toolSet.Defs)),
		attribute.Int("denied.count", len(deniedTools)),
	)

	return toolSet, deniedTools, nil
}

func (b *Builder) addToolDefs(toolSet *conversation.ToolSet, tool conversation.Tool, def *conversation.ToolDefinition) {
	for i := range def.Tool {
		funcName := def.Tool[i].Function.Name
		if _, exists := toolSet.ByName[funcName]; !exists {
			toolSet.ByName[funcName] = tool
			toolSet.Defs = append(toolSet.Defs, def.Tool[i])
		}
	}
	if len(def.Instructions) > 0 {
		toolSet.Instructions = append(toolSet.Instructions, def.Instructions...)
	}
}
