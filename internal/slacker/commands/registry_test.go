package commands

import (
	"context"
	"testing"
)

func TestNormalizeInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase",
			input:    "HELLO",
			expected: "hello",
		},
		{
			name:     "multiple spaces",
			input:    "hello   world",
			expected: "hello world",
		},
		{
			name:     "tabs and spaces",
			input:    "hello\t\tworld",
			expected: "hello world",
		},
		{
			name:     "leading and trailing spaces",
			input:    "  hello  ",
			expected: "hello",
		},
		{
			name:     "mixed case and spaces",
			input:    "  HeLLo   WoRLD  ",
			expected: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeInput(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeInput(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCommandRegistry_Match_LongestPrefix(t *testing.T) {
	t.Parallel()
	registry := NewCommandRegistry()

	registry.Register("model", func(ctx context.Context, deps *CommandsDependencies, msg string) error {
		return nil
	})
	registry.Register("model access", func(ctx context.Context, deps *CommandsDependencies, msg string) error {
		return nil
	})

	cmd, _, found := registry.Match("model access list")
	if !found {
		t.Fatal("expected to find match")
	}
	if cmd != "model access" {
		t.Errorf("expected longest match 'model access', got %q", cmd)
	}
}

func TestCommandRegistry_Match_CaseInsensitive(t *testing.T) {
	t.Parallel()
	registry := NewCommandRegistry()

	registry.Register("help", func(ctx context.Context, deps *CommandsDependencies, msg string) error {
		return nil
	})

	tests := []struct {
		name  string
		input string
		want  string
		found bool
	}{
		{"lowercase", "help", "help", true},
		{"uppercase", "HELP", "help", true},
		{"mixed case", "Help", "help", true},
		{"no match", "helo", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, found := registry.Match(tt.input)
			if found != tt.found {
				t.Errorf("Match(%q) found = %v, want %v", tt.input, found, tt.found)
			}
			if cmd != tt.want {
				t.Errorf("Match(%q) = %q, want %q", tt.input, cmd, tt.want)
			}
		})
	}
}

func TestCommandRegistry_Match_WhitespaceNormalization(t *testing.T) {
	t.Parallel()
	registry := NewCommandRegistry()

	registry.Register("reset session", func(ctx context.Context, deps *CommandsDependencies, msg string) error {
		return nil
	})

	tests := []struct {
		name  string
		input string
		want  string
		found bool
	}{
		{"single space", "reset session", "reset session", true},
		{"multiple spaces", "reset   session", "reset session", true},
		{"tabs", "reset\tsession", "reset session", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, found := registry.Match(tt.input)
			if found != tt.found {
				t.Errorf("Match(%q) found = %v, want %v", tt.input, found, tt.found)
			}
			if cmd != tt.want {
				t.Errorf("Match(%q) = %q, want %q", tt.input, cmd, tt.want)
			}
		})
	}
}

func TestCommandRegistry_EmptyInput(t *testing.T) {
	t.Parallel()
	registry := NewCommandRegistry()

	registry.Register("help", func(ctx context.Context, deps *CommandsDependencies, msg string) error {
		return nil
	})

	_, _, found := registry.Match("")
	if found {
		t.Error("expected no match for empty input")
	}

	_, _, found = registry.Match("   ")
	if found {
		t.Error("expected no match for whitespace-only input")
	}
}
