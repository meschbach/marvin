package slacker

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// IntentProcessor handles natural language intent recognition for tool management
type IntentProcessor struct {
	patterns  []IntentPattern
	threshold float64
}

// IntentPattern represents a pattern for matching user intents
type IntentPattern struct {
	Pattern    string
	Action     string
	ToolType   string
	Confidence float64
}

// ToolManagementIntent represents a detected intent for tool management
type ToolManagementIntent struct {
	Action     string // "add", "share", "list", "remove"
	ToolType   string // "http", "local", "docker"
	Config     interface{}
	TargetUser string // for sharing actions
	Target     string // for other targeting
	Confidence float64
	Original   string // original message
}

// NewIntentProcessor creates a new intent processor with default patterns
func NewIntentProcessor() *IntentProcessor {
	return &IntentProcessor{
		threshold: 0.7,
		patterns: []IntentPattern{
			{
				Pattern:    `(?i)add (?:http|mcp) (?:server|tool) (?:at|from) (.+)`,
				Action:     "add_tool",
				ToolType:   "http",
				Confidence: 0.9,
			},
			{
				Pattern:    `(?i)add (?:local|local program) (?:tool|at)?\s*(.+)`,
				Action:     "add_tool",
				ToolType:   "local",
				Confidence: 0.8,
			},
			{
				Pattern:    `(?i)add (?:docker|container) (?:tool|mcp|server) (.+)`,
				Action:     "add_tool",
				ToolType:   "docker",
				Confidence: 0.8,
			},
			{
				Pattern:    `(?i)share (.+) with @?([^\s]+)`,
				Action:     "share_tool",
				Confidence: 0.85,
			},
			{
				Pattern:    `(?i)share (.+) to @?([^\s]+)`,
				Action:     "share_tool",
				Confidence: 0.85,
			},
			{
				Pattern:    `(?i)list (?:my )?tools`,
				Action:     "list_tools",
				Confidence: 0.9,
			},
			{
				Pattern:    `(?i)what tools (?:do i have|can i use)`,
				Action:     "list_tools",
				Confidence: 0.85,
			},
			{
				Pattern:    `(?i)remove (?:tool )?(.+)`,
				Action:     "remove_tool",
				Confidence: 0.7,
			},
			{
				Pattern:    `(?i)delete (?:tool )?(.+)`,
				Action:     "remove_tool",
				Confidence: 0.7,
			},
			{
				Pattern:    `(?i)approve\s+([a-zA-Z0-9-]+)`,
				Action:     "approve_tool",
				Confidence: 0.9,
			},
			{
				Pattern:    `(?i)reject\s+([a-zA-Z0-9-]+)(?::\s*(.+))?`,
				Action:     "reject_tool",
				Confidence: 0.9,
			},
			{
				Pattern:    `(?i)reset (?:my )?(?:session|context|conversation)`,
				Action:     "reset_session",
				Confidence: 0.9,
			},
		},
	}
}

// ProcessMessage analyzes a message and returns the detected intent
func (ip *IntentProcessor) ProcessMessage(message string) (*ToolManagementIntent, error) {
	message = strings.TrimSpace(message)

	var bestMatch *ToolManagementIntent
	highestConfidence := 0.0

	for _, pattern := range ip.patterns {
		re, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			continue // Skip invalid patterns
		}

		matches := re.FindStringSubmatch(message)
		if matches == nil {
			continue
		}

		// Only consider if confidence meets threshold
		if pattern.Confidence < ip.threshold {
			continue
		}

		// Check if this is the best match so far
		if pattern.Confidence <= highestConfidence {
			continue
		}

		intent := &ToolManagementIntent{
			Action:     pattern.Action,
			ToolType:   pattern.ToolType,
			Confidence: pattern.Confidence,
			Original:   message,
		}

		// Extract parameters based on action
		switch pattern.Action {
		case "add_tool":
			if len(matches) > 1 {
				intent.Config = strings.TrimSpace(matches[1])
			}
		case "share_tool":
			if len(matches) > 2 {
				intent.Target = strings.TrimSpace(matches[1])
				intent.TargetUser = strings.TrimSpace(matches[2])
				// Remove @ prefix if present
				intent.TargetUser = strings.TrimPrefix(intent.TargetUser, "@")
			}
		case "remove_tool":
			if len(matches) > 1 {
				intent.Target = strings.TrimSpace(matches[1])
			}
		case "approve_tool":
			if len(matches) > 1 {
				intent.Target = strings.TrimSpace(matches[1])
			}
		case "reject_tool":
			if len(matches) > 1 {
				intent.Target = strings.TrimSpace(matches[1])
			}
			if len(matches) > 2 && matches[2] != "" {
				intent.Config = strings.TrimSpace(matches[2])
			}
		}

		bestMatch = intent
		highestConfidence = pattern.Confidence
	}

	return bestMatch, nil
}

// GenerateToolID generates a unique tool ID
func GenerateToolID(userID, toolType, name string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("user_%s_%s_%s_%d", userID, toolType, name, timestamp)
}

// GenerateApprovalRequestID generates a unique approval request ID
func GenerateApprovalRequestID(toolType, userID string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s-req-%d-%s", toolType, timestamp, userID[:8])
}
