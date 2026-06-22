package query

import (
	"context"
	"fmt"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	llmfactory "github.com/meschbach/marvin/internal/llm/factory"
)

// LoadToolsByNames loads only the specified tools from the configuration.
// It returns an error if any requested tool is not found.
func LoadToolsByNames(ctx context.Context, cfg *config.File, toolNames []string) (*conversation.ToolSet, error) {
	ts := conversation.NewToolSet()
	var errs []error

	if cfg == nil || len(toolNames) == 0 {
		return ts, nil
	}

	// Build lookup map for requested tools
	requested := make(map[string]bool)
	for _, name := range toolNames {
		requested[name] = true
	}

	// Load LocalPrograms that match requested tools
	for _, lp := range cfg.LocalPrograms {
		if requested[lp.Name] {
			tool := FromLocalProgram(lp)
			ts.Container.Register(tool)
			if err := ts.RegisterTool(ctx, tool); err != nil {
				errs = append(errs, fmt.Errorf("local program %q: %w", lp.Name, err))
			}
			delete(requested, lp.Name)
		}
	}

	// Load DockerMCP tools that match
	for _, mcp := range cfg.DockerMCPBlock {
		if requested[mcp.Name] {
			tool := FromDockerSpec(mcp)
			ts.Container.Register(tool)
			if err := ts.RegisterTool(ctx, tool); err != nil {
				errs = append(errs, fmt.Errorf("docker mcp %q: %w", mcp.Name, err))
			}
			delete(requested, mcp.Name)
		}
	}

	// Load HTTPMCP tools that match
	for _, mcp := range cfg.HttpMCPBlock {
		if requested[mcp.Name] {
			tool := FromHTTPMCPService(mcp)
			ts.Container.Register(tool)
			if err := ts.RegisterTool(ctx, tool); err != nil {
				errs = append(errs, fmt.Errorf("http mcp %q: %w", mcp.Name, err))
			}
			delete(requested, mcp.Name)
		}
	}

	// Handle "rag" - load all documents if requested
	if requested["rag"] && len(cfg.Documents) > 0 {
		embedder, err := llmfactory.NewEmbeddingProvider(ctx, cfg)
		if err != nil {
			return ts, fmt.Errorf("creating embedding provider for RAG: %w", err)
		}
		for _, rag := range cfg.Documents {
			tool := NewChromemTool(rag, embedder, false)
			if err := ts.RegisterTool(ctx, tool); err != nil {
				errs = append(errs, fmt.Errorf("rag tool %q: %w", rag.Name, err))
			}
		}
		delete(requested, "rag")
	}

	// Check for any remaining requested tools that weren't found
	for name := range requested {
		errs = append(errs, fmt.Errorf("tool %q not found in configuration", name))
	}

	if len(errs) > 0 {
		return ts, fmt.Errorf("tool loading errors: %v", errs)
	}

	return ts, nil
}

func loadToolsFromConfig(ctx context.Context, cfg *config.File) (*conversation.ToolSet, []error) {
	ts := conversation.NewToolSet()
	var warnings []error
	if cfg != nil {
		for _, lp := range cfg.LocalPrograms {
			t := FromLocalProgram(lp)
			ts.Container.Register(t)
			if err := ts.RegisterTool(ctx, t); err != nil {
				warnings = append(warnings, &localProgramDiscoveryError{
					name:       t.Name,
					underlying: err,
				})
				continue
			}
		}
		if err := loadToolsFromDocker(ctx, ts, cfg); err != nil {
			warnings = append(warnings, err)
		}
		if err := loadToolsFromHTTP(ctx, ts, cfg); err != nil {
			warnings = append(warnings, err)
		}
	}
	return ts, warnings
}
