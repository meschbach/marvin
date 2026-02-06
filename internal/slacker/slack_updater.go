package slacker

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/slack-go/slack"
)

// updaterState represents the current state of the updater
type updaterState int

const (
	updaterStateThinking updaterState = iota
	updaterStateContent
	updaterStateComplete
)

// SlackSink provides an abstraction for Slack message operations.
// It enables testing by allowing mock implementations and supports different
// Slack client strategies while maintaining a consistent interface for
// real-time message operations.
type SlackSink interface {
	UpdateMessageContext(ctx context.Context, channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error)
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
}

// SlackUpdater provides real-time progress updates for long-running AI operations in Slack.
// It uses a simplified state-based approach with automatic flushing on transitions.
type SlackUpdater struct {
	client       SlackSink
	channelID    string
	messageTS    string
	currentState updaterState
	buffer       strings.Builder
	toolCalls    []string
	mutex        sync.Mutex
}

// NewSlackUpdater creates an updater that provides visibility into AI operations.
func NewSlackUpdater(client SlackSink, channelID string) *SlackUpdater {
	return &SlackUpdater{
		client:       client,
		channelID:    channelID,
		currentState: updaterStateThinking,
		buffer:       strings.Builder{},
		toolCalls:    []string{},
		mutex:        sync.Mutex{},
	}
}

// setState transitions to a new state and flushes the current buffer if needed
// NOTE: Assumed invoking goproc has locked the updater
func (su *SlackUpdater) setState(ctx context.Context, newState updaterState) error {
	if su.currentState == newState {
		return nil
	}

	// Flush current buffer before transitioning
	if su.buffer.Len() > 0 {
		if err := su.postMessage(ctx); err != nil {
			return err
		}
		// Reset buffer for new content
		su.buffer.Reset()
	}

	su.currentState = newState
	return nil
}

// formatMessage creates the formatted message based on current state
func (su *SlackUpdater) formatMessage() string {
	var message strings.Builder

	content := su.buffer.String()
	if content == "" {
		return ""
	}

	switch su.currentState {
	case updaterStateThinking:
		// Use italic formatting for thinking
		message.WriteString(fmt.Sprintf("_%s_", content))
	case updaterStateContent, updaterStateComplete:
		// Regular formatting for content
		message.WriteString(content)
	}

	return message.String()
}

// postMessage posts the current buffer as a new message
// NOTE: caller is expected to hold the mutex
func (su *SlackUpdater) postMessage(ctx context.Context) error {
	message := su.formatMessage()
	if message == "" {
		return nil
	}

	// Parse message to blocks
	parser := NewSlackParser()
	blocks := parser.ParseMessageToBlocks(message)

	var err error
	if su.messageTS == "" {
		// Post new message
		_, su.messageTS, err = su.client.PostMessageContext(
			ctx,
			su.channelID,
			slack.MsgOptionBlocks(blocks...),
		)
	} else {
		// Update existing message
		_, _, _, err = su.client.UpdateMessageContext(
			ctx,
			su.channelID,
			su.messageTS,
			slack.MsgOptionBlocks(blocks...),
		)
	}

	return err
}

// postToolCalls creates a separate message for tool calls
func (su *SlackUpdater) postToolCalls(ctx context.Context) error {
	if len(su.toolCalls) == 0 {
		return nil
	}

	var toolMessage strings.Builder
	toolMessage.WriteString("🔧 Tools used: ")
	for i, tool := range su.toolCalls {
		if i > 0 {
			toolMessage.WriteString(", ")
		}
		toolMessage.WriteString(fmt.Sprintf("`%s`", tool))
	}

	// Parse tool message to blocks
	parser := NewSlackParser()
	blocks := parser.ParseMessageToBlocks(toolMessage.String())

	var err error
	if su.messageTS == "" {
		// Post new message
		_, su.messageTS, err = su.client.PostMessageContext(
			ctx,
			su.channelID,
			slack.MsgOptionBlocks(blocks...),
		)
	} else {
		// Update existing message
		_, _, _, err = su.client.UpdateMessageContext(
			ctx,
			su.channelID,
			su.messageTS,
			slack.MsgOptionBlocks(blocks...),
		)
	}

	return err
}

// AddContent adds content and transitions to content state if needed
func (su *SlackUpdater) AddContent(ctx context.Context, content string) error {
	su.mutex.Lock()
	defer su.mutex.Unlock()

	// Transition to content state if needed
	if su.currentState != updaterStateContent && su.currentState != updaterStateComplete {
		if err := su.setState(ctx, updaterStateContent); err != nil {
			return fmt.Errorf("failed to transition to content state: %w", err)
		}
	}

	su.buffer.WriteString(content)
	return nil
}

// AddThought adds thinking content and transitions to thinking state if needed
func (su *SlackUpdater) AddThought(ctx context.Context, thought string) error {
	su.mutex.Lock()
	defer su.mutex.Unlock()

	// Transition to thinking state if needed
	if su.currentState != updaterStateThinking {
		if err := su.setState(ctx, updaterStateThinking); err != nil {
			return fmt.Errorf("failed to transition to thinking state: %w", err)
		}
	}

	su.buffer.WriteString(thought)
	return nil
}

// AddToolCall records a tool call and posts it immediately
func (su *SlackUpdater) AddToolCall(ctx context.Context, toolName string) error {
	su.mutex.Lock()
	defer su.mutex.Unlock()

	su.toolCalls = append(su.toolCalls, toolName)
	if err := su.postToolCalls(ctx); err != nil {
		return fmt.Errorf("failed to post tool call: %w", err)
	}
	return nil
}

// Complete transitions to complete state and posts final message
func (su *SlackUpdater) Complete(ctx context.Context) error {
	su.mutex.Lock()
	defer su.mutex.Unlock()

	// Always flush final buffer when completing, even if already in correct state
	if su.buffer.Len() > 0 {
		if err := su.postMessage(ctx); err != nil {
			return err
		}
		// Reset buffer after final flush
		su.buffer.Reset()
	}

	su.currentState = updaterStateComplete
	return nil
}

// ForceUpdate provides compatibility with existing code - posts current buffer
func (su *SlackUpdater) ForceUpdate(ctx context.Context) error {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	return su.postMessage(ctx)
}

// getBufferContent returns current buffer content for debugging
func (su *SlackUpdater) getBufferContent() (string, string, []string) {
	su.mutex.Lock()
	defer su.mutex.Unlock()

	var thinkingContent, contentContent string
	currentContent := su.buffer.String()

	if su.currentState == updaterStateThinking {
		thinkingContent = currentContent
	} else {
		contentContent = currentContent
	}

	return contentContent, thinkingContent, su.toolCalls
}
