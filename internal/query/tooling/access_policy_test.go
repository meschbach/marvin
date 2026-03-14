package tooling

import (
	"context"
	"testing"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
)

type mockPolicyTool struct {
	name string
}

func (m *mockPolicyTool) DefineAPI(_ context.Context) (*conversation.ToolDefinition, error) {
	return &conversation.ToolDefinition{
		Tool: []api.Tool{
			{
				Type: conversation.ToolTypeFunction,
				Function: api.ToolFunction{
					Name:        m.name,
					Description: "A test tool",
				},
			},
		},
	}, nil
}

func (m *mockPolicyTool) Invoke(_ context.Context, _ api.ToolCall) ([]api.Message, error) {
	return nil, nil
}

func TestAccessPolicy_IsAdmin(t *testing.T) {
	t.Parallel()
	policy := NewAccessPolicy([]string{"admin1", "admin2"})

	assert.True(t, policy.IsAdmin("admin1"), "admin1 should be admin")
	assert.True(t, policy.IsAdmin("admin2"), "admin2 should be admin")
	assert.False(t, policy.IsAdmin("user1"), "user1 should not be admin")
	assert.False(t, policy.IsAdmin(""), "empty user should not be admin")
}

func TestAccessPolicy_CanAccess_AdminBypasses(t *testing.T) {
	t.Parallel()
	policy := NewAccessPolicy([]string{"admin1"})

	restrictedTool := &ToolWithMetadata{
		Tool:         &mockPolicyTool{name: "restricted"},
		AllowedUsers: []string{"user1"},
	}

	assert.True(t, policy.CanAccess(t.Context(), "admin1", restrictedTool), "admin should bypass restrictions")
}

func TestAccessPolicy_CanAccess_NoSharingConfig(t *testing.T) {
	t.Parallel()
	policy := NewAccessPolicy([]string{})

	openTool := &ToolWithMetadata{
		Tool:         &mockPolicyTool{name: "open"},
		AllowedUsers: nil,
	}

	assert.True(t, policy.CanAccess(t.Context(), "user1", openTool), "nil AllowedUsers means open to all")
	assert.True(t, policy.CanAccess(t.Context(), "user2", openTool), "nil AllowedUsers means open to all")
}

func TestAccessPolicy_CanAccess_AllowedUsers(t *testing.T) {
	t.Parallel()
	policy := NewAccessPolicy([]string{})

	restrictedTool := &ToolWithMetadata{
		Tool:         &mockPolicyTool{name: "restricted"},
		AllowedUsers: []string{"user1", "user2"},
	}

	assert.True(t, policy.CanAccess(t.Context(), "user1", restrictedTool), "allowed user should have access")
	assert.True(t, policy.CanAccess(t.Context(), "user2", restrictedTool), "allowed user should have access")
}

func TestAccessPolicy_CanAccess_NonAllowedUsersDenied(t *testing.T) {
	t.Parallel()
	policy := NewAccessPolicy([]string{})

	restrictedTool := &ToolWithMetadata{
		Tool:         &mockPolicyTool{name: "restricted"},
		AllowedUsers: []string{"user1", "user2"},
	}

	assert.False(t, policy.CanAccess(t.Context(), "user3", restrictedTool), "user3 should be denied")
	assert.False(t, policy.CanAccess(t.Context(), "user4", restrictedTool), "user4 should be denied")
}

func TestAccessPolicy_CanAccess_EmptyAllowedUsers(t *testing.T) {
	t.Parallel()
	policy := NewAccessPolicy([]string{})

	emptyRestrictedTool := &ToolWithMetadata{
		Tool:         &mockPolicyTool{name: "empty_restricted"},
		AllowedUsers: []string{},
	}

	assert.False(t, policy.CanAccess(t.Context(), "user1", emptyRestrictedTool), "empty AllowedUsers means no access")
}

func TestAccessPolicy_UniformRules(t *testing.T) {
	t.Parallel()
	policy := NewAccessPolicy([]string{})

	restrictedTool := &ToolWithMetadata{
		Tool:         &mockPolicyTool{name: "test"},
		AllowedUsers: []string{"U123", "U456"},
	}

	assert.True(t, policy.CanAccess(t.Context(), "U123", restrictedTool), "U123 is allowed")
	assert.True(t, policy.CanAccess(t.Context(), "U456", restrictedTool), "U456 is allowed")
	assert.False(t, policy.CanAccess(t.Context(), "U789", restrictedTool), "U789 is denied")
}
