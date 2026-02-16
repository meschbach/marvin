package query

import (
	"context"
	"fmt"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"
)

// CLIStreamingUpdater provides streaming updates for the command-line interface.
// It handles debug flags and statistics tracking to maintain current CLI behavior.
//
// Example:
//
//	updater := NewCLIStreamingUpdater(true, false, true)
//	engine := NewEngine(client, config, logger, tools, messages)
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

	// thinkingFormat determines how thinking content is formatted
	thinkingFormat string

	// Statistics tracking (cumulative across the entire conversation)
	promptTokens   int
	responseTokens int
	totalTokens    int
}

// NewCLIStreamingUpdater creates a new CLI streaming updater with the specified debug options
//
// Example:
//
//	updater := NewCLIStreamingUpdater(true, false, true, "markdown")
//	engine := NewEngine(client, config, logger, tools, messages)
//	err := engine.RunConversation(ctx, model, updater)
//
// The updater will display thinking, tool calls, and final statistics
// according to the enabled debug flags and thinking format.
func NewCLIStreamingUpdater(showThinking, showTools, showDone bool, thinkingFormat string) *CLIStreamingUpdater {
	return &CLIStreamingUpdater{
		showThinking:   showThinking,
		showTools:      showTools,
		showDone:       showDone,
		thinkingFormat: thinkingFormat,
	}
}

// AddContent streams content directly to console
func (c *CLIStreamingUpdater) AddContent(ctx context.Context, content string) error {
	fmt.Print(content)
	return nil
}

// AddThought streams thinking content if debug mode is enabled
func (c *CLIStreamingUpdater) AddThought(ctx context.Context, thought string) error {
	if !c.showThinking {
		return nil
	}

	switch c.thinkingFormat {
	case "markdown":
		// Simple markdown formatting - just add a header
		fmt.Printf("## 🤔 Thinking\n%s", thought)
	case "collapsed":
		// Collapsed format - brief prefix
		fmt.Printf("🤔 Thinking: %s", thought)
	default: // "plain"
		// Plain format - existing behavior
		fmt.Printf("Thinking: %s", thought)
	}
	return nil
}

// AddToolCall logs tool calls if debug mode is enabled
func (c *CLIStreamingUpdater) AddToolCall(ctx context.Context, toolCall api.ToolCall) error {
	if c.showTools {
		// Note: Detailed tool call info will be logged by the engine
		fmt.Printf("🔧 Tool call: %s: %#v\n", toolCall.Function.Name, toolCall.Function.Arguments.All())
		for key, value := range toolCall.Function.Arguments.All() {
			fmt.Printf("\t-\t%s: %#v\n", key, value)
		}
	}
	return nil
}

// AddToolResult logs tool execution results if debug mode is enabled
func (c *CLIStreamingUpdater) AddToolResult(ctx context.Context, toolCall api.ToolCall, result []api.Message, err error) error {
	if c.showTools {
		if err != nil {
			fmt.Printf("❌ Tool %s failed: %v\n", toolCall.Function.Name, err)
		} else {
			fmt.Printf("✅ Tool %s completed\n", toolCall.Function.Name)
			for _, msg := range result {
				if msg.Content != "" {
					fmt.Printf("\tResult: %s\n", msg.Content)
				}
			}
		}
	}
	return nil
}

// UpdateStats tracks real-time statistics from the LLM
func (c *CLIStreamingUpdater) UpdateStats(ctx context.Context, stats conversation.ConversationStats) error {
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
