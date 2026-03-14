package tooling

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_Build_Idempotent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	b := NewBuilder()
	registry := NewRegistry()

	tool1 := &testMockTool{name: "tool_a", description: "Tool A", toolCount: 1}
	tool2 := &testMockTool{name: "tool_b", description: "Tool B", toolCount: 1}
	registry.Register(ctx, tool1, nil)
	registry.Register(ctx, tool2, nil)

	policy := NewAccessPolicy(nil)
	userCtx := &UserInfo{UserID: "user1"}

	result1, err := b.Build(ctx, userCtx, registry, policy)
	require.NoError(t, err)

	result2, err := b.Build(ctx, userCtx, registry, policy)
	require.NoError(t, err)

	assert.Len(t, result1.Defs, len(result2.Defs), "should return same number of tools")
}

func TestBuilder_Build_AliasNoDuplicate(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	b := NewBuilder()
	registry := NewRegistry()

	tool1 := &testMockTool{name: "tool_one", description: "Tool One", toolCount: 1}
	tool2 := &testMockTool{name: "tool_two", description: "Tool Two", toolCount: 1}
	registry.Register(ctx, tool1, nil)
	registry.Register(ctx, tool2, nil)

	policy := NewAccessPolicy(nil)
	userCtx := &UserInfo{UserID: "user1"}

	result, err := b.Build(ctx, userCtx, registry, policy)
	require.NoError(t, err)

	assert.Len(t, result.Defs, 2, "should have 2 unique tools")
}

func TestBuilder_Build_SameToolMultipleDefinitions(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	b := NewBuilder()
	registry := NewRegistry()

	multiTool := &testMockTool{name: "multi", description: "Multi-tool", toolCount: 3}
	registry.Register(ctx, multiTool, nil)

	policy := NewAccessPolicy(nil)
	userCtx := &UserInfo{UserID: "user1"}

	result, err := b.Build(ctx, userCtx, registry, policy)
	require.NoError(t, err)

	assert.Len(t, result.Defs, 3, "should have 3 tool definitions")
}

func TestBuilder_BuildWithDeniedInfo_Idempotent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	b := NewBuilder()
	registry := NewRegistry()

	restrictedTool := &testMockTool{name: "restricted", description: "Restricted", toolCount: 1}
	registry.Register(ctx, restrictedTool, []string{"user2"})

	policy := NewAccessPolicy(nil)
	userCtx := &UserInfo{UserID: "user1"}

	result1, denied1, err := b.BuildWithDeniedInfo(ctx, userCtx, registry, policy)
	require.NoError(t, err)

	result2, denied2, err := b.BuildWithDeniedInfo(ctx, userCtx, registry, policy)
	require.NoError(t, err)

	assert.Len(t, result1.Defs, len(result2.Defs), "should return same tools")
	assert.Equal(t, denied1, denied2, "should return same denied list")
}

func TestBuilder_BuildWithDeniedInfo_ReturnsDeniedList(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	b := NewBuilder()
	registry := NewRegistry()

	restrictedTool := &testMockTool{name: "restricted", description: "Restricted", toolCount: 1}
	registry.Register(ctx, restrictedTool, []string{"user2"})

	policy := NewAccessPolicy(nil)
	userCtx := &UserInfo{UserID: "user1"}

	result, denied, err := b.BuildWithDeniedInfo(ctx, userCtx, registry, policy)
	require.NoError(t, err)

	assert.Empty(t, result.Defs, "user1 should have no tools")
	assert.Contains(t, denied, "restricted", "restricted tool should be in denied list")
}

func TestBuilder_Build_AdminAccessAll(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	b := NewBuilder()
	registry := NewRegistry()

	restrictedTool := &testMockTool{name: "restricted", description: "Restricted", toolCount: 1}
	registry.Register(ctx, restrictedTool, []string{"user2"})

	policy := NewAccessPolicy([]string{"admin1"})
	userCtx := &UserInfo{UserID: "admin1"}

	result, err := b.Build(ctx, userCtx, registry, policy)
	require.NoError(t, err)

	assert.Len(t, result.Defs, 1, "admin should have access to restricted tool")
}

func TestBuilder_Build_EmptyRegistry(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	b := NewBuilder()
	registry := NewRegistry()

	policy := NewAccessPolicy(nil)
	userCtx := &UserInfo{UserID: "user1"}

	result, err := b.Build(ctx, userCtx, registry, policy)
	require.NoError(t, err)

	assert.Empty(t, result.Defs, "empty registry should return empty toolset")
}
