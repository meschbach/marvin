package tooling

import (
	"context"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/junk"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ToolFactory creates tool instances from configuration blocks.
// Implementations handle the specifics of instantiating HTTP, local program, or Docker MCP tools.
type ToolFactory interface {
	CreateHTTPTool(block *config.HttpMCPBlock) conversation.Tool
	CreateLocalProgramTool(block config.LocalProgramBlock) conversation.Tool
	CreateDockerTool(block *config.DockerMCPBlock) conversation.Tool
}

// Loader loads and initializes tools from configuration into a Registry.
// Handles HTTP MCP tools, local programs, and Docker MCP tools, including
// validation of tool APIs and tracking of per-tool allowed users.
type Loader struct {
	container   *junk.Container
	toolFactory ToolFactory
}

// NewLoader creates a new Loader instance with the given container and tool factory.
// The container provides runtime resources; the factory creates tool instances.
func NewLoader(container *junk.Container, factory ToolFactory) *Loader {
	return &Loader{
		container:   container,
		toolFactory: factory,
	}
}

// LoadTools parses the configuration and loads all configured tools into a new Registry.
// Validates each tool's API definition and tracks allowed users for access control.
// Returns the populated Registry, any warnings (e.g., load failures, empty allowed lists),
// and an error if loading fails catastrophically.
func (l *Loader) LoadTools(ctx context.Context, cfg *config.File) (*Registry, []string, error) {
	ctx, span := tracer.Start(ctx, "ToolLoader.LoadTools", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	reg := NewRegistry()
	var warnings []string

	for _, httpCfg := range cfg.HttpMCPBlock {
		l.loadSingleTool(ctx, httpCfg.Name, "HTTP", httpCfg.URL, httpCfg.Sharing,
			func() conversation.Tool { return l.toolFactory.CreateHTTPTool(httpCfg) },
			reg, &warnings)
	}

	for _, localCfg := range cfg.LocalPrograms {
		l.loadSingleTool(ctx, localCfg.Name, "local_program", localCfg.Program, localCfg.Sharing,
			func() conversation.Tool { return l.toolFactory.CreateLocalProgramTool(localCfg) },
			reg, &warnings)
	}

	for _, dockerCfg := range cfg.DockerMCPBlock {
		l.loadSingleTool(ctx, dockerCfg.Name, "docker_mcp", dockerCfg.Image, dockerCfg.Sharing,
			func() conversation.Tool { return l.toolFactory.CreateDockerTool(dockerCfg) },
			reg, &warnings)
	}

	span.SetAttributes(attribute.Int("init.warnings.count", len(warnings)))
	return reg, warnings, nil
}

func (l *Loader) loadSingleTool(
	ctx context.Context,
	toolName, toolType, toolSource string,
	sharing *config.SharingBlock,
	createTool func() conversation.Tool,
	reg *Registry,
	warnings *[]string,
) {
	ctx, toolSpan := tracer.Start(ctx, "ToolLoader.loadSingle",
		trace.WithAttributes(
			attribute.String("tool.name", toolName),
			attribute.String("tool.type", toolType),
			attribute.String("tool.source", toolSource),
		),
	)
	defer toolSpan.End()

	tool := createTool()
	_, err := tool.DefineAPI(ctx)
	if err != nil {
		junk.RecordSpanErrorNoLint(toolSpan, err)
		*warnings = append(*warnings, fmt.Sprintf("%s tool %q failed: %v", toolType, toolName, err))
		return
	}

	var allowedUsers []string
	if sharing != nil {
		if len(sharing.AllowedUsers) == 0 {
			*warnings = append(*warnings, fmt.Sprintf("%s %q has empty allowed_users - no access will be granted", toolType, toolName))
		} else {
			allowedUsers = sharing.AllowedUsers
		}
	}

	l.registerToolDefs(ctx, reg, tool, toolName, allowedUsers, warnings)
}

func (l *Loader) registerToolDefs(ctx context.Context, reg *Registry, tool conversation.Tool, toolName string, allowedUsers []string, warnings *[]string) {
	definition, err := tool.DefineAPI(ctx)
	if err != nil {
		return
	}

	for i := range definition.Tool {
		toolDef := definition.Tool[i]
		if existing, exists := reg.Get(toolDef.Function.Name); exists {
			*warnings = append(*warnings,
				fmt.Sprintf("Tool name collision: %q being loaded from %q overwrites previous tool", toolDef.Function.Name, toolName))
			_ = existing
		}

		reg.RegisterToolDef(ctx, tool, toolDef.Function.Name, allowedUsers)
	}
}
