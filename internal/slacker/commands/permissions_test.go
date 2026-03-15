package commands

import "testing"

func TestIsAdminCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cmdName  string
		expected bool
	}{
		{
			name:     "admin command",
			cmdName:  "admin",
			expected: true,
		},
		{
			name:     "model access command",
			cmdName:  "model access",
			expected: true,
		},
		{
			name:     "add tool command",
			cmdName:  "add tool",
			expected: true,
		},
		{
			name:     "remove tool command",
			cmdName:  "remove tool",
			expected: true,
		},
		{
			name:     "help command",
			cmdName:  "help",
			expected: false,
		},
		{
			name:     "tools command",
			cmdName:  "tools",
			expected: false,
		},
		{
			name:     "reset session command",
			cmdName:  "reset session",
			expected: false,
		},
		{
			name:     "unknown command",
			cmdName:  "unknown",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAdminCommand(tt.cmdName)
			if result != tt.expected {
				t.Errorf("isAdminCommand(%q) = %v, want %v", tt.cmdName, result, tt.expected)
			}
		})
	}
}
