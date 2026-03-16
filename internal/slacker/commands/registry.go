package commands

import (
	"context"
	"strings"
	"unicode"
)

type CommandHandler func(ctx context.Context, deps *CommandsDependencies, msg string) error

type CommandRegistry struct {
	handlers         map[string]CommandHandler
	adminCommandsMap map[string]bool
}

func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		handlers:         make(map[string]CommandHandler),
		adminCommandsMap: adminCommands,
	}
}

func (r *CommandRegistry) Register(name string, handler CommandHandler) {
	r.handlers[name] = handler
}

func (r *CommandRegistry) Match(input string) (string, CommandHandler, bool) {
	normalized := normalizeInput(input)
	if normalized == "" {
		return "", nil, false
	}

	longestMatch := ""
	var handler CommandHandler

	for cmd := range r.handlers {
		if strings.HasPrefix(normalized, cmd) {
			if len(cmd) > len(longestMatch) {
				longestMatch = cmd
				handler = r.handlers[cmd]
			}
		}
	}

	if handler != nil {
		return longestMatch, handler, true
	}
	return "", nil, false
}

func normalizeInput(input string) string {
	input = strings.ToLower(input)
	input = compressWhitespace(input)
	return strings.TrimSpace(input)
}

func compressWhitespace(s string) string {
	var result []rune
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				result = append(result, ' ')
				prevSpace = true
			}
		} else {
			result = append(result, r)
			prevSpace = false
		}
	}
	return string(result)
}

var adminCommands = map[string]bool{
	"admin":        true,
	"model access": true,
	"add tool":     true,
	"remove tool":  true,
}

func isAdminCommand(name string) bool {
	return adminCommands[name]
}
