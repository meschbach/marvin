package tooling

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_Get_NotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	_, found := r.Get("nonexistent")
	assert.False(t, found, "should not find tool that was never registered")
}

func TestRegistry_All_Empty(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	all := r.All()
	assert.Empty(t, all, "fresh registry should have no tools")
}

func TestRegistry_Register_SingleTool(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	r := NewRegistry()

	tool := &testMockTool{name: "test_tool", description: "A test tool", toolCount: 1}
	r.Register(ctx, tool, nil)

	found, ok := r.Get("test_tool")
	require.True(t, ok, "should find registered tool")
	assert.Equal(t, tool, found.Tool)
	assert.Nil(t, found.AllowedUsers)
}

func TestRegistry_Register_MultipleTools(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	r := NewRegistry()

	tool := &testMockTool{name: "multi", description: "Multi-tool", toolCount: 3}
	r.Register(ctx, tool, []string{"user1", "user2"})

	all := r.All()
	assert.Len(t, all, 3, "should have 3 tools registered")

	for _, name := range []string{"multi_a", "multi_b", "multi_c"} {
		found, ok := r.Get(name)
		require.True(t, ok, "should find tool %s", name)
		assert.Equal(t, tool, found.Tool)
		assert.Equal(t, []string{"user1", "user2"}, found.AllowedUsers)
	}
}

func TestRegistry_All_ReturnsCopy(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	r := NewRegistry()

	tool := &testMockTool{name: "test_tool", description: "A test tool", toolCount: 1}
	r.Register(ctx, tool, nil)

	all1 := r.All()
	all2 := r.All()

	assert.Equal(t, all1, all2, "All should return equivalent maps")
	all1["test_tool"] = &ToolWithMetadata{}
	assert.NotEqual(t, all1, all2, "modifying returned map should not affect original")
}

func TestRegistry_Register_WithAllowedUsers(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	r := NewRegistry()

	allowedUsers := []string{"user1", "user2", "user3"}
	tool := &testMockTool{name: "restricted_tool", description: "Restricted tool", toolCount: 1}
	r.Register(ctx, tool, allowedUsers)

	found, ok := r.Get("restricted_tool")
	require.True(t, ok)
	assert.Equal(t, allowedUsers, found.AllowedUsers)
}
