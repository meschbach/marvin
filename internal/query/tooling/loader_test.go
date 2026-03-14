package tooling

import (
	"context"
	"fmt"
	"testing"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockToolFactory struct{}

func (m *mockToolFactory) CreateHTTPTool(block *config.HttpMCPBlock) conversation.Tool {
	return &failingTool{name: block.Name, toolType: "HTTP tool"}
}

func (m *mockToolFactory) CreateLocalProgramTool(block config.LocalProgramBlock) conversation.Tool {
	return &failingTool{name: block.Name, toolType: "local_program tool"}
}

func (m *mockToolFactory) CreateDockerTool(block *config.DockerMCPBlock) conversation.Tool {
	return &failingTool{name: block.Name, toolType: "docker_mcp tool"}
}

type failingTool struct {
	name     string
	toolType string
}

func (f *failingTool) DefineAPI(_ context.Context) (*conversation.ToolDefinition, error) {
	return nil, fmt.Errorf("%s %q failed", f.toolType, f.name)
}

func (f *failingTool) Invoke(_ context.Context, _ api.ToolCall) ([]api.Message, error) {
	return nil, nil
}

func TestLoader_LoadTools_EmptyConfig(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	l := NewLoader(nil, &mockToolFactory{})

	cfg := &config.File{}
	reg, warnings, err := l.LoadTools(ctx, cfg)

	require.NoError(t, err)
	assert.Empty(t, reg.All(), "empty config should have no tools")
	assert.Empty(t, warnings, "empty config should have no warnings")
}

func TestLoader_LoadTools_InvalidLocalPrograms(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	l := NewLoader(nil, &mockToolFactory{})

	cfg := &config.File{
		LocalPrograms: []config.LocalProgramBlock{
			{
				Name:    "invalid1",
				Program: "/nonexistent/program",
			},
			{
				Name:    "invalid2",
				Program: "/another/missing/program",
			},
		},
	}

	_, warnings, err := l.LoadTools(ctx, cfg)

	require.NoError(t, err, "load should succeed despite invalid tools")
	assert.Len(t, warnings, 2, "expected two warnings for invalid local programs")

	for _, w := range warnings {
		assert.Contains(t, w, "local_program tool", "warning should mention tool type")
	}
}

func TestLoader_LoadTools_InvalidTools(t *testing.T) {
	t.Parallel()
	l := NewLoader(nil, &mockToolFactory{})

	tests := []struct {
		name        string
		cfg         *config.File
		wantLen     int
		containsStr string
	}{
		{
			name: "HTTP tool failure",
			cfg: &config.File{
				HttpMCPBlock: []*config.HttpMCPBlock{
					{
						Name: "invalid_http",
						URL:  "http://this-is-not-a-valid-server:12345",
					},
				},
			},
			wantLen:     1,
			containsStr: "HTTP tool",
		},
		{
			name: "Docker tool failure",
			cfg: &config.File{
				DockerMCPBlock: []*config.DockerMCPBlock{
					{
						Name:  "invalid_docker",
						Image: "this-image-does-not-exist:latest",
					},
				},
			},
			wantLen:     1,
			containsStr: "docker_mcp tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			_, warnings, err := l.LoadTools(ctx, tt.cfg)

			require.NoError(t, err, "load should succeed despite invalid tools")
			assert.Len(t, warnings, tt.wantLen, "expected warning for invalid tool")
			assert.Contains(t, warnings[0], tt.containsStr, "warning should mention tool type")
		})
	}
}
