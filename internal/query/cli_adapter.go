package query

import (
	"context"
	"fmt"

	"github.com/ollama/ollama/api"
)

// CLIStreamingUpdater provides streaming updates for the command-line interface.
// It handles debug flags and statistics tracking to maintain current CLI behavior.
//
// Example:
//
//	updater := NewCLIStreamingUpdater(true, false, true)
//	engine := NewConversationEngine(client, config, logger, tools, messages)
//	err := engine.RunConversation(ctx, model, updater)
//
// The updater will display thinking, tool calls, and final statistics
// according to the enabled debug flags.
type CLIStreamingUpdater struct {
	// showThinking determines if AI reasoning should be displayed
	showThinking bool

	// showTools determines if tool invocation details should be logged
	showTools bool

	// showDone determines if completion messages should be displayed
	showDone bool

	// Statistics tracking (cumulative across the entire conversation)
	promptTokens   int
	responseTokens int
	totalTokens    int
}

// NewCLIStreamingUpdater creates a new CLI streaming updater with the specified debug options
//
// Example:
//
//	updater := NewCLIStreamingUpdater(true, false, true)
//	engine := NewConversationEngine(client, config, logger, tools, messages)
//	err := engine.RunConversation(ctx, model, updater)
//
// The updater will display thinking, tool calls, and final statistics
// according to the enabled debug flags.
func NewCLIStreamingUpdater(showThinking, showTools, showDone bool) *CLIStreamingUpdater {
	return &CLIStreamingUpdater{
		showThinking: showThinking,
		showTools:    showTools,
		showDone:     showDone,
	}
}

// AddContent streams content directly to console
func (c *CLIStreamingUpdater) AddContent(ctx context.Context, content string) error {
	fmt.Print(content)
	return nil
}

// AddThought streams thinking content if debug mode is enabled
func (c *CLIStreamingUpdater) AddThought(ctx context.Context, thought string) error {
	if c.showThinking {
		fmt.Printf("Thinking: %s", thought)
	}
	return nil
}

// AddToolCall logs tool calls if debug mode is enabled
func (c *CLIStreamingUpdater) AddToolCall(ctx context.Context, toolCall api.ToolCall) error {
	if c.showTools {
		// Note: Detailed tool call info will be logged by the engine
		fmt.Printf("🔧 Tool call: %s\n", toolCall.Function.Name)
		for key, value := range toolCall.Function.Arguments.All() {
			fmt.Printf("\t-\t%s: %#v\n", key, value)
		}
	}
	return nil
}

// UpdateStats tracks real-time statistics from the LLM
func (c *CLIStreamingUpdater) UpdateStats(ctx context.Context, stats ConversationStats) error {
	// Update cumulative statistics
	c.promptTokens = stats.PromptTokens
	c.responseTokens = stats.ResponseTokens
	c.totalTokens = stats.TotalTokens

	// Show completion message if enabled
	if stats.IsDone && c.showDone {
		fmt.Printf("<Done> (%d) %s\n", stats.EvalCount, stats.DoneReason)
	}

	return nil
}

// Flush outputs final statistics and any remaining content
func (c *CLIStreamingUpdater) Flush(ctx context.Context) error {
	fmt.Printf("\nTotal tokens: %d = (prompt tokens: %d) + (response tokens: %d)\n",
		c.totalTokens, c.promptTokens, c.responseTokens)
	return nil
}
