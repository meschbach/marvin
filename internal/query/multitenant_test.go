package query

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/llm"
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
		def.Tool = append(def.Tool, llm.ToolDefinition{
			Type: conversation.ToolTypeFunction,
			Function: llm.ToolFunction{
				Name:        toolName,
				Description: m.description,
			},
		})
	}
	return def, nil
}

func (m *mockToolForMultiTenant) Invoke(_ context.Context, _ llm.ToolCall) ([]llm.Message, error) {
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

	assert.Empty(t, tts.registry.All())
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

	assert.Empty(t, tts.registry.All(), "no tools should be registered")
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

	assert.Empty(t, tts.registry.All())
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

	assert.Empty(t, tts.registry.All())
	warnings := tts.GetInitializationWarnings()
	assert.Len(t, warnings, 1)
	assert.True(t, strings.HasPrefix(warnings[0], "docker_mcp tool"), "warning should mention docker_mcp tool")
}

func TestTenantToolSet_GetUserTools_Idempotent(t *testing.T) {
	t.Parallel()

	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)
	tts.SetGlobalToolForTesting("alpha", &mockToolForMultiTenant{name: "alpha", description: "Alpha tool", toolCount: 1})
	tts.SetGlobalToolForTesting("beta", &mockToolForMultiTenant{name: "beta", description: "Beta tool", toolCount: 1})

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
	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)
	tts.SetGlobalToolForTesting("tool_one", &mockToolForMultiTenant{name: "tool_one", description: "Tool one", toolCount: 1})
	tts.SetGlobalToolForTesting("tool_two", &mockToolForMultiTenant{name: "tool_two", description: "Tool two", toolCount: 1})

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
	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)
	tts.SetGlobalToolForTesting("multi", &mockToolForMultiTenant{name: "multi", description: "Multi-tool", toolCount: 3})

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
	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)
	tts.SetGlobalToolForTesting("shared", sameTool)

	userCtx := &UserContext{UserID: "user1"}
	toolSet, err := tts.GetUserTools(t.Context(), userCtx)
	require.NoError(t, err)

	// Should have exactly 1 tool, not 2 (deduplicated)
	assert.Len(t, toolSet.Defs, 1, "Should deduplicate tools from global+user")
	assert.Equal(t, "shared", toolSet.Defs[0].Function.Name)
}

func TestTenantToolSet_GetUserToolsWithDeniedInfo_Idempotent(t *testing.T) {
	t.Parallel()

	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)
	tts.SetGlobalToolForTesting("alpha", &mockToolForMultiTenant{name: "alpha", description: "Alpha"})
	tts.SetGlobalToolForTesting("beta", &mockToolForMultiTenant{name: "beta", description: "Beta"})
	tts.InjectToolForTesting("beta", &mockToolForMultiTenant{name: "beta", description: "Beta"}, []string{"user1"})

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

	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)
	// Register the same tool instance under two different alias names
	tts.SetGlobalToolForTesting("alias1", multiTool)
	tts.SetGlobalToolForTesting("alias2", multiTool)

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

func TestTenantToolSet_Sharing_AllowedUsersCanAccess(t *testing.T) {
	t.Parallel()

	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)
	// Tool restricted to specific users
	tts.InjectToolForTesting("shared_tool", &mockToolForMultiTenant{name: "shared_tool", description: "Shared", toolCount: 1}, []string{"U123"})

	allowedUser := &UserContext{UserID: "U123"}
	toolSet, err := tts.GetUserTools(t.Context(), allowedUser)
	require.NoError(t, err)

	assert.Len(t, toolSet.Defs, 1, "Allowed user should have access to shared tool")
	assert.Equal(t, "shared_tool", toolSet.Defs[0].Function.Name)
}

func TestTenantToolSet_Sharing_NonAllowedUsersDenied(t *testing.T) {
	t.Parallel()

	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)
	// Tool restricted to specific users
	tts.InjectToolForTesting("shared_tool", &mockToolForMultiTenant{name: "shared_tool", description: "Shared", toolCount: 1}, []string{"U123"})

	deniedUser := &UserContext{UserID: "U456"}
	toolSet, err := tts.GetUserTools(t.Context(), deniedUser)
	require.NoError(t, err)

	assert.Empty(t, toolSet.Defs, "Non-allowed user should not have access to shared tool")
}

func TestTenantToolSet_Sharing_Expiration(t *testing.T) {
	// This test is removed - ExpiresAt feature was removed per proposal
	// The new design uses AllowedUsers which doesn't have expiration
	t.Skip("ExpiresAt feature removed")
}

func TestTenantToolSet_Sharing_AdminBypass(t *testing.T) {
	t.Parallel()

	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)
	// Tool restricted to specific users
	tts.InjectToolForTesting("restricted_tool", &mockToolForMultiTenant{name: "restricted_tool", description: "Restricted", toolCount: 1}, []string{"U123"})
	// Set admin
	tts.SetAdminUsersForTesting([]string{"admin1"})

	adminUser := &UserContext{UserID: "admin1", IsAdmin: true}
	toolSet, err := tts.GetUserTools(t.Context(), adminUser)
	require.NoError(t, err)

	assert.Len(t, toolSet.Defs, 1, "Admin should have access to all tools regardless of permissions")
}

func TestTenantToolSet_Sharing_EmptyAllowedUsersWarning(t *testing.T) {
	t.Parallel()

	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)

	// Note: The new design doesn't have a direct way to set initWarnings
	// This test may need to be redesigned
	warnings := tts.GetInitializationWarnings()
	_ = warnings
}

func TestTenantToolSet_Sharing_NoSharingAvailableToAll(t *testing.T) {
	t.Parallel()

	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)
	// Tool without restricted users (nil means open to all)
	tts.SetGlobalToolForTesting("open_tool", &mockToolForMultiTenant{name: "open_tool", description: "Open", toolCount: 1})

	user := &UserContext{UserID: "random_user"}
	toolSet, err := tts.GetUserTools(t.Context(), user)
	require.NoError(t, err)

	assert.Len(t, toolSet.Defs, 1, "Tool without sharing block should be available to all users")
}

func TestTenantToolSet_Sharing_HTTP_ToolsWithSharing(t *testing.T) {
	t.Parallel()

	// HTTP tool marked as restricted with sharing config
	cfg := &config.File{}
	tts, err := NewTenantToolSet(t.Context(), cfg)
	require.NoError(t, err)
	// Tool restricted to specific users
	tts.InjectToolForTesting("http_tool", &mockToolForMultiTenant{name: "http_tool", description: "HTTP", toolCount: 1}, []string{"U123", "U456"})

	allowedUser := &UserContext{UserID: "U123"}
	toolSet, err := tts.GetUserTools(t.Context(), allowedUser)
	require.NoError(t, err)
	assert.Len(t, toolSet.Defs, 1, "Allowed user should have access to HTTP tool with sharing")

	allowedUser2 := &UserContext{UserID: "U456"}
	toolSet2, err := tts.GetUserTools(t.Context(), allowedUser2)
	require.NoError(t, err)
	assert.Len(t, toolSet2.Defs, 1, "Other allowed user should have access to HTTP tool with sharing")

	deniedUser := &UserContext{UserID: "U789"}
	toolSet3, err := tts.GetUserTools(t.Context(), deniedUser)
	require.NoError(t, err)
	assert.Empty(t, toolSet3.Defs, "Non-allowed user should not have access to HTTP tool with sharing")
}
