package query

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockToolForMultiTenant is a test tool implementation for TenantToolSet tests
type mockToolForMultiTenant struct {
	name        string
	description string
	toolCount   int // number of tools to return from DefineAPI
}

func (m *mockToolForMultiTenant) DefineAPI(_ context.Context) (*conversation.ToolDefinition, error) {
	def := &conversation.ToolDefinition{}
	for i := 0; i < m.toolCount; i++ {
		toolName := m.name
		if m.toolCount > 1 {
			toolName = m.name + "_" + string(rune('a'+i))
		}
		def.Tool = append(def.Tool, api.Tool{
			Type: conversation.ToolTypeFunction,
			Function: api.ToolFunction{
				Name:        toolName,
				Description: m.description,
			},
		})
	}
	return def, nil
}

func (m *mockToolForMultiTenant) Invoke(_ context.Context, _ api.ToolCall) ([]api.Message, error) {
	return nil, nil
}

func TestTenantToolSet_GetInitializationWarnings(t *testing.T) {
	t.Parallel()
	tts := &TenantToolSet{}
	warnings := tts.GetInitializationWarnings()
	assert.Empty(t, warnings, "should be empty for fresh TenantToolSet")
}

func TestTenantToolSet_Initialize_NoTools(t *testing.T) {
	t.Parallel()
	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)

	err = tts.Initialize(t.Context())
	require.NoError(t, err)

	assert.Empty(t, tts.globalTools)
	warnings := tts.GetInitializationWarnings()
	assert.Empty(t, warnings)
}

func TestTenantToolSet_Initialize_AllInvalidLocalPrograms(t *testing.T) {
	t.Parallel()
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
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)

	err = tts.Initialize(t.Context())
	require.NoError(t, err, "Initialize should not fail despite all tools failing")

	assert.Empty(t, tts.globalTools, "no tools should be registered")
	warnings := tts.GetInitializationWarnings()
	assert.Len(t, warnings, 2, "expected two warnings")
	for _, w := range warnings {
		assert.True(t, strings.HasPrefix(w, "local_program tool"), "warning should mention tool type")
	}
}

func TestTenantToolSet_Initialize_AllInvalidHTTP(t *testing.T) {
	t.Parallel()
	cfg := &config.File{
		HttpMCPBlock: []*config.HttpMCPBlock{
			{
				Name: "invalid_http",
				URL:  "http://this-is-not-a-valid-server:12345",
			},
		},
	}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)

	err = tts.Initialize(t.Context())
	require.NoError(t, err)

	assert.Empty(t, tts.globalTools)
	warnings := tts.GetInitializationWarnings()
	assert.Len(t, warnings, 1)
	assert.True(t, strings.HasPrefix(warnings[0], "HTTP tool"), "warning should mention HTTP tool")
}

func TestTenantToolSet_Initialize_AllInvalidDocker(t *testing.T) {
	t.Parallel()
	cfg := &config.File{
		DockerMCPBlock: []*config.DockerMCPBlock{
			{
				Name:  "invalid_docker",
				Image: "this-image-does-not-exist:latest",
				AssistantPrompt: &config.AssistantPromptBlock{
					FromString: "you are a helpful assistant",
				},
			},
		},
	}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)

	err = tts.Initialize(t.Context())
	require.NoError(t, err)

	assert.Empty(t, tts.globalTools)
	warnings := tts.GetInitializationWarnings()
	assert.Len(t, warnings, 1)
	assert.True(t, strings.HasPrefix(warnings[0], "docker_mcp tool"), "warning should mention docker_mcp tool")
}

func TestTenantToolSet_GetUserTools_Idempotent(t *testing.T) {
	t.Parallel()

	tts := &TenantToolSet{
		globalTools: map[string]conversation.Tool{
			"tool_a": &mockToolForMultiTenant{name: "alpha", description: "Alpha tool", toolCount: 1},
			"tool_b": &mockToolForMultiTenant{name: "beta", description: "Beta tool", toolCount: 1},
		},
	}

	userCtx := &UserContext{UserID: "user1"}

	// Call GetUserTools multiple times
	toolSet1, err := tts.GetUserTools(t.Context(), userCtx)
	require.NoError(t, err)

	toolSet2, err := tts.GetUserTools(t.Context(), userCtx)
	require.NoError(t, err)

	// Both calls should return the same number of tools
	assert.Equal(t, len(toolSet1.Defs), len(toolSet2.Defs),
		"Multiple GetUserTools calls should return same tool count")

	// Verify tool names match
	names1 := make([]string, len(toolSet1.Defs))
	names2 := make([]string, len(toolSet2.Defs))
	for i, t := range toolSet1.Defs {
		names1[i] = t.Function.Name
	}
	for i, t := range toolSet2.Defs {
		names2[i] = t.Function.Name
	}
	assert.ElementsMatch(t, names1, names2, "Tool names should match between calls")
}

func TestTenantToolSet_GetUserTools_AliasNoDuplicate(t *testing.T) {
	t.Parallel()

	// Same tool registered under two different names (aliases)
	// Each alias returns a DIFFERENT tool name to test if duplicates are being added
	tts := &TenantToolSet{
		globalTools: map[string]conversation.Tool{
			"alias_one": &mockToolForMultiTenant{name: "tool_one", description: "Tool one", toolCount: 1},
			"alias_two": &mockToolForMultiTenant{name: "tool_two", description: "Tool two", toolCount: 1},
		},
	}

	userCtx := &UserContext{UserID: "user1"}
	toolSet, err := tts.GetUserTools(t.Context(), userCtx)
	require.NoError(t, err)

	// Should have 2 unique tools, NOT 4 (duplicate entries of each)
	assert.Len(t, toolSet.Defs, 2, "Should have 2 unique tools from 2 aliases")

	// Verify unique tool names - should be tool_one and tool_two
	names := make(map[string]bool)
	for _, t := range toolSet.Defs {
		names[t.Function.Name] = true
	}
	assert.Len(t, names, 2, "Should have 2 unique tool names")
	assert.True(t, names["tool_one"], "Should have tool_one")
	assert.True(t, names["tool_two"], "Should have tool_two")
}

func TestTenantToolSet_GetUserTools_SameToolMultipleDefinitions(t *testing.T) {
	t.Parallel()

	// A tool that returns multiple tool definitions from a single DefineAPI call
	tts := &TenantToolSet{
		globalTools: map[string]conversation.Tool{
			"multi_tool": &mockToolForMultiTenant{name: "multi", description: "Multi-tool", toolCount: 3},
		},
	}

	userCtx := &UserContext{UserID: "user1"}
	toolSet, err := tts.GetUserTools(t.Context(), userCtx)
	require.NoError(t, err)

	// Should have exactly 3 tools from the one tool definition
	assert.Len(t, toolSet.Defs, 3, "Should have 3 tools from multi-definition tool")

	// Verify all three are present
	names := make([]string, len(toolSet.Defs))
	for i, t := range toolSet.Defs {
		names[i] = t.Function.Name
	}
	assert.ElementsMatch(t, []string{"multi_a", "multi_b", "multi_c"}, names)
}

func TestTenantToolSet_GetUserTools_GlobalAndUserOverlap(t *testing.T) {
	t.Parallel()

	// Same tool instance in both global and user-specific maps
	sameTool := &mockToolForMultiTenant{name: "shared", description: "Shared tool", toolCount: 1}
	tts := &TenantToolSet{
		globalTools: map[string]conversation.Tool{
			"shared": sameTool,
		},
		userTools: map[string]map[string]conversation.Tool{
			"user1": {
				"shared": sameTool, // Same instance
			},
		},
	}

	userCtx := &UserContext{UserID: "user1"}
	toolSet, err := tts.GetUserTools(t.Context(), userCtx)
	require.NoError(t, err)

	// Should have exactly 1 tool, not 2 (deduplicated)
	assert.Len(t, toolSet.Defs, 1, "Should deduplicate tools from global+user")
	assert.Equal(t, "shared", toolSet.Defs[0].Function.Name)
}

func TestTenantToolSet_GetUserToolsWithDeniedInfo_Idempotent(t *testing.T) {
	t.Parallel()

	tts := &TenantToolSet{
		globalTools: map[string]conversation.Tool{
			"tool_a": &mockToolForMultiTenant{name: "alpha", description: "Alpha"},
			"tool_b": &mockToolForMultiTenant{name: "beta", description: "Beta"},
		},
		permissions: map[string]*ToolPermission{
			"tool_b:user1": {ToolID: "tool_b", UserID: "user1", CanInvoke: true},
		},
	}

	userCtx := &UserContext{UserID: "user1"}

	// Call multiple times
	toolSet1, denied1, err := tts.GetUserToolsWithDeniedInfo(t.Context(), userCtx)
	require.NoError(t, err)

	toolSet2, denied2, err := tts.GetUserToolsWithDeniedInfo(t.Context(), userCtx)
	require.NoError(t, err)

	// Should be identical
	assert.Equal(t, len(toolSet1.Defs), len(toolSet2.Defs))
	assert.ElementsMatch(t, denied1, denied2)
}

func TestTenantToolSet_GetUserTools_DefineAPIPurity(t *testing.T) {
	t.Parallel()

	// Create a tool that returns multiple tools
	tool := &mockToolForMultiTenant{name: "multi", description: "Multi", toolCount: 3}
	ctx := t.Context()

	// Call DefineAPI twice
	def1, err := tool.DefineAPI(ctx)
	require.NoError(t, err)
	def2, err := tool.DefineAPI(ctx)
	require.NoError(t, err)

	// Verify they are different slices (not sharing underlying array)
	// by modifying one and checking the other is unchanged
	if len(def1.Tool) > 0 && len(def2.Tool) > 0 {
		originalName := def2.Tool[0].Function.Name
		def1.Tool[0].Function.Name = "modified"
		assert.NotEqual(t, "modified", def2.Tool[0].Function.Name,
			"DefineAPI should return independent slices; modifying one should not affect the other")
		// Restore for cleanup
		def1.Tool[0].Function.Name = originalName
	}

	// Verify they are different slice headers (length check ensures they're separate allocations)
	assert.NotEqual(t, fmt.Sprintf("%p", def1.Tool), fmt.Sprintf("%p", def2.Tool),
		"DefineAPI should return different slice instances each call")
}

func TestTenantToolSet_GetUserTools_MultiToolAliasOverlap(t *testing.T) {
	t.Parallel()

	// A tool that returns 3 definitions: "multi_a", "multi_b", "multi_c"
	multiTool := &mockToolForMultiTenant{name: "multi", description: "Multi-tool", toolCount: 3}

	// Register the same tool instance under two different alias names
	tts := &TenantToolSet{
		globalTools: map[string]conversation.Tool{
			"alias1": multiTool,
			"alias2": multiTool, // Same instance, different key
		},
	}

	userCtx := &UserContext{UserID: "user1"}
	toolSet, err := tts.GetUserTools(t.Context(), userCtx)
	require.NoError(t, err)

	// Should have exactly 3 unique tools, not 6 (3 from first alias + 3 from second)
	assert.Len(t, toolSet.Defs, 3, "Should have 3 tools, not duplicated due to alias overlap")

	// Verify we have the three expected tool names
	names := make(map[string]bool)
	for _, t := range toolSet.Defs {
		names[t.Function.Name] = true
	}
	expected := []string{"multi_a", "multi_b", "multi_c"}
	for _, n := range expected {
		assert.True(t, names[n], "Should contain %s", n)
	}
	assert.Len(t, names, 3, "Should have exactly 3 unique tool names")
}
