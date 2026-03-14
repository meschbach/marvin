package tooling

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AccessPolicy controls access to tools based on admin users and per-tool allowed user lists.
// Admins bypass all access restrictions. For non-admins, access is granted only if the tool
// has no allowed users list (open access) or if the user is explicitly listed.
type AccessPolicy struct {
	adminUsers map[string]bool
}

// NewAccessPolicy creates a new AccessPolicy with the given list of admin user IDs.
// Admin users bypass all tool access restrictions.
func NewAccessPolicy(adminUsers []string) *AccessPolicy {
	users := make(map[string]bool, len(adminUsers))
	for _, u := range adminUsers {
		users[u] = true
	}
	return &AccessPolicy{
		adminUsers: users,
	}
}

// IsAdmin returns true if the given user ID is configured as an admin.
// Admin users bypass all tool access restrictions.
func (p *AccessPolicy) IsAdmin(userID string) bool {
	return p.adminUsers[userID]
}

// CanAccess determines whether a user can access a specific tool.
// Returns true if: the user is an admin, the tool has no allowed users list (open access),
// or the user is explicitly listed in the tool's allowed users. Otherwise returns false.
func (p *AccessPolicy) CanAccess(ctx context.Context, userID string, tool *ToolWithMetadata) bool {
	_, span := tracer.Start(ctx, "AccessPolicy.CanAccess", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("tool.name", getToolName(tool)),
	)

	if p.IsAdmin(userID) {
		span.SetAttributes(attribute.Bool("access.allowed", true))
		return true
	}

	if tool.AllowedUsers == nil {
		span.SetAttributes(attribute.Bool("access.allowed", true))
		return true
	}

	for _, u := range tool.AllowedUsers {
		if u == userID {
			span.SetAttributes(attribute.Bool("access.allowed", true))
			return true
		}
	}

	span.SetAttributes(attribute.Bool("access.allowed", false))
	return false
}

func getToolName(tool *ToolWithMetadata) string {
	if tool == nil || tool.Tool == nil {
		return ""
	}
	return "tool"
}
