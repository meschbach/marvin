package slacker

import (
	"testing"
)

func TestIntentProcessor_DoesNotMatchAdmin(t *testing.T) {
	processor := NewIntentProcessor()

	messages := []string{
		"admin",
		"admin help",
		"admin list pending requests",
		"admin escalate something",
	}

	for _, msg := range messages {
		intent, err := processor.ProcessMessage(msg)
		if err != nil {
			t.Fatalf("ProcessMessage(%q) returned error: %v", msg, err)
		}
		if intent != nil {
			t.Errorf("ProcessMessage(%q) unexpectedly matched intent: %+v", msg, intent)
		}
	}
}

func TestIntentProcessor_DoesNotMatchAdminEscalationPattern(t *testing.T) {
	processor := NewIntentProcessor()

	// Ensure admin_escalation pattern is not present
	for _, pattern := range processor.patterns {
		if pattern.Action == "admin_escalation" || pattern.Action == "admin_help" {
			t.Errorf("Found admin-related pattern: %+v", pattern)
		}
	}
}
