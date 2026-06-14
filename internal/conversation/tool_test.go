package conversation

import (
	"context"
	"testing"

	"github.com/meschbach/marvin/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTool struct {
	name        string
	description string
}

func (m *mockTool) DefineAPI(_ context.Context) (*ToolDefinition, error) {
	return &ToolDefinition{
		Tool: []llm.ToolDefinition{
			{
				Type: ToolTypeFunction,
				Function: llm.ToolFunction{
					Name:        m.name,
					Description: m.description,
				},
			},
		},
	}, nil
}

func (m *mockTool) Invoke(_ context.Context, _ llm.ToolCall) ([]llm.Message, error) {
	return nil, nil
}

func TestToolSet_RegisterTool_DeduplicatesByName(t *testing.T) {
	t.Parallel()

	ts := NewToolSet()
	ctx := t.Context()

	err := ts.RegisterTool(ctx, &mockTool{name: "test_tool", description: "A test tool"})
	require.NoError(t, err)
	assert.Len(t, ts.Defs, 1, "should have one tool after first registration")

	err = ts.RegisterTool(ctx, &mockTool{name: "test_tool", description: "A test tool - duplicate"})
	require.NoError(t, err, "registering same tool name should not error")
	assert.Len(t, ts.Defs, 1, "should still have only one tool after duplicate registration")
	assert.Equal(t, "A test tool", ts.Defs[0].Function.Description, "original tool should be preserved")
}

func TestToolSet_RegisterTool_MultipleDifferentTools(t *testing.T) {
	t.Parallel()

	ts := NewToolSet()
	ctx := t.Context()

	err := ts.RegisterTool(ctx, &mockTool{name: "tool_one", description: "First tool"})
	require.NoError(t, err)

	err = ts.RegisterTool(ctx, &mockTool{name: "tool_two", description: "Second tool"})
	require.NoError(t, err)

	assert.Len(t, ts.Defs, 2, "should have two different tools")
}

func TestToolSet_APITools_ReturnsUniqueTools(t *testing.T) {
	t.Parallel()

	ts := NewToolSet()
	ctx := t.Context()

	require.NoError(t, ts.RegisterTool(ctx, &mockTool{name: "echo", description: "Echo tool"}))
	require.NoError(t, ts.RegisterTool(ctx, &mockTool{name: "echo", description: "Another echo tool"}))

	tools := ts.APITools()
	seen := make(map[string]bool)
	for _, tool := range tools {
		if seen[tool.Function.Name] {
			t.Errorf("duplicate tool found: %s", tool.Function.Name)
		}
		seen[tool.Function.Name] = true
	}
}

func TestToolSet_ByName_UniqueMapping(t *testing.T) {
	t.Parallel()

	ts := NewToolSet()
	ctx := t.Context()

	require.NoError(t, ts.RegisterTool(ctx, &mockTool{name: "my_tool", description: "First"}))
	require.NoError(t, ts.RegisterTool(ctx, &mockTool{name: "my_tool", description: "Second"}))

	tool, exists := ts.ByName["my_tool"]
	require.True(t, exists, "tool should exist in ByName map")
	assert.NotNil(t, tool, "tool should not be nil")
}
