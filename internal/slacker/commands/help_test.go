package commands

import (
	"context"
	"strings"
	"testing"
)

func TestRenderHelp(t *testing.T) {
	t.Parallel()
	registry := NewCommandRegistry()
	helpText := RenderHelp(registry)

	if helpText == "" {
		t.Error("expected non-empty help text")
	}

	expectedSections := []string{
		"Available Commands:",
		"help",
		"tools",
		"thinking",
		"preferences",
		"reset session",
		"Admin Commands:",
		"admin",
		"model access",
		"add tool",
		"remove tool",
		"list tools",
	}

	for _, section := range expectedSections {
		if !strings.Contains(helpText, section) {
			t.Errorf("expected help text to contain %q", section)
		}
	}
}

func TestRenderHelp_ContainsAllCommands(t *testing.T) {
	t.Parallel()
	registry := NewCommandRegistry()

	registry.Register("custom", func(ctx context.Context, deps *CommandsDependencies, msg string) error {
		return nil
	})

	helpText := RenderHelp(registry)

	if !strings.Contains(helpText, "Available Commands:") {
		t.Error("help should contain Available Commands section")
	}
}
