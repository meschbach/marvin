package query

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/junk"
)

// FromHTTPMCPService connects to a remote HTTP API
func FromHTTPMCPService(block *config.HttpMCPBlock) *Mark3labsTool {
	spec := &httpMCPSpec{
		config: block,
	}
	return &Mark3labsTool{
		Name:            block.Name,
		spec:            spec,
		assistantPrompt: block.AssistantPrompt,
	}
}

type httpMCPSpec struct {
	config *config.HttpMCPBlock
}

func (h httpMCPSpec) start(ctx context.Context) (runningProgram, error) {
	streamingTransport, err := transport.NewStreamableHTTP(h.config.URL)
	if err != nil {
		return nil, err
	}

	return &httpMCPEndpoint{
		config:     h.config,
		connection: streamingTransport,
	}, nil
}

type httpMCPEndpoint struct {
	config     *config.HttpMCPBlock
	connection transport.Interface
}

func (h *httpMCPEndpoint) transport() transport.Interface {
	return h.connection
}

func (h *httpMCPEndpoint) stop(ctx context.Context) error {
	return h.connection.Close()
}

func loadToolsFromHTTP(ctx context.Context, ts *conversation.ToolSet, cfg *config.File) (problem error) {
	for _, mcpCfg := range cfg.HttpMCPBlock {
		tool := FromHTTPMCPService(mcpCfg)
		ts.Container.Register(tool)
		if err := ts.RegisterTool(ctx, tool); err != nil {
			return &junk.OperationalError{
				Description: fmt.Sprintf("failed to Register %s", mcpCfg.Name),
				Underlying:  err,
			}
		}
	}
	return problem
}
