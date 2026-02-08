package slacker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/meschbach/marvin/internal/query"
	"github.com/ollama/ollama/api"
	"github.com/slack-go/slack"
)

// updaterState represents the current state of the updater
type updaterState int

func (u updaterState) toContentType() ContentType {
	switch u {
	case updaterStateInit:
		return ContentIgnore
	case updaterStateThinking:
		return ContentThinking
	case updaterStateContent:
		return ContentOutput
	case updaterStateTool:
		return ContentTool
	case updaterStateComplete:
		return ContentIgnore
	}
	panic(fmt.Sprintf("unknown updater state: %d", u))
}

const (
	updaterStateInit updaterState = iota
	updaterStateThinking
	updaterStateContent
	updaterStateTool
	updaterStateComplete
)

type ContentType int

const (
	ContentOutput ContentType = iota
	ContentThinking
	ContentTool
	ContentIgnore
)

// SlackSink provides an abstraction for Slack message operations.
// It enables testing by allowing mock implementations and supports different
// Slack client strategies while maintaining a consistent interface for
// real-time message operations.
type SlackSink interface {
	UpdateMessageContext(ctx context.Context, channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error)
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
}

type ContentFormatter interface {
	Format(ctx context.Context, content string, contentType ContentType) ([]slack.Block, error)
}

// TimeProvider interface for deterministic testing
type TimeProvider interface {
	Now() time.Time
}

// DefaultTimeProvider uses system time
type DefaultTimeProvider struct{}

func (d *DefaultTimeProvider) Now() time.Time {
	return time.Now()
}

// SlackUpdaterOption configures SlackUpdater behavior
type SlackUpdaterOption func(*SlackUpdater)

// WithTimeProvider injects custom time provider for testing
func WithTimeProvider(timeProvider TimeProvider) SlackUpdaterOption {
	return func(su *SlackUpdater) {
		su.timeProvider = timeProvider
	}
}

// SlackUpdater provides real-time progress updates for long-running AI operations in Slack.
// It uses time-based buffering with automatic flushing on state transitions.
type SlackUpdater struct {
	client         SlackSink
	channelID      string
	messageTS      string
	currentState   updaterState
	buffer         strings.Builder
	mutex          sync.Mutex
	lastUpdateTime time.Time
	lastWritten    string
	formatter      ContentFormatter
	timeProvider   TimeProvider
}

// NewSlackUpdater creates an updater that provides visibility into AI operations.
func NewSlackUpdater(client SlackSink, channelID string, formatter ContentFormatter, options ...SlackUpdaterOption) *SlackUpdater {
	su := &SlackUpdater{
		client:         client,
		channelID:      channelID,
		currentState:   updaterStateInit,
		buffer:         strings.Builder{},
		mutex:          sync.Mutex{},
		lastUpdateTime: time.Time{},
		formatter:      formatter,
		timeProvider:   &DefaultTimeProvider{},
	}

	// Apply options
	for _, option := range options {
		option(su)
	}

	return su
}

// switchToType transitions to a new state and flushes the current buffer immediately
// NOTE: Assumed invoking goproc has locked the updater
func (su *SlackUpdater) switchToType(ctx context.Context, newState updaterState) (changed bool, err error) {
	if su.currentState == newState {
		return false, nil
	}

	// Post the current buffer immediately on type change
	err = su.updateMessage(ctx)
	su.messageTS = ""
	su.buffer.Reset()
	su.lastUpdateTime = su.timeProvider.Now()

	su.currentState = newState
	return true, err
}

// addContentInternal handles the core logic for adding content to the updater
func (su *SlackUpdater) addContentInternal(
	ctx context.Context,
	content string,
	targetState updaterState,
) error {
	su.mutex.Lock()
	defer su.mutex.Unlock()

	// Switch content type if needed (posts previous buffer immediately)
	changed, switchErr := su.switchToType(ctx, targetState)
	su.buffer.WriteString(content)

	// Check time-based update condition: same type AND >1 second since last update
	timeSinceLastUpdate := su.timeProvider.Now().Sub(su.lastUpdateTime)
	var progressError error
	if changed || timeSinceLastUpdate > time.Second {
		progressError = su.updateMessage(ctx)
		su.lastUpdateTime = su.timeProvider.Now()
	}

	return errors.Join(switchErr, progressError)
}

// updateMessage posts or updates the current buffer content
// NOTE: caller is expected to hold the mutex
func (su *SlackUpdater) updateMessage(ctx context.Context) error {
	message := su.buffer.String()
	if message == "" {
		return nil
	}

	// Parse message to blocks
	blocks, err := su.formatter.Format(ctx, message, su.currentState.toContentType())
	if err != nil {
		return err
	}

	if su.messageTS == "" {
		// Post new message
		_, su.messageTS, err = su.client.PostMessageContext(
			ctx,
			su.channelID,
			slack.MsgOptionBlocks(blocks...),
		)
		su.lastWritten = message
	} else {
		if su.lastWritten == message {
			return nil
		}
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
	return su.addContentInternal(ctx, content, updaterStateContent)
}

// AddThought adds thinking content and transitions to thinking state if needed
func (su *SlackUpdater) AddThought(ctx context.Context, thought string) error {
	return su.addContentInternal(ctx, thought, updaterStateThinking)
}

// AddToolCall records a tool call and treats it as regular content
func (su *SlackUpdater) AddToolCall(ctx context.Context, toolCall api.ToolCall) error {
	// Tool calls now treated as regular content with thinking-style formatting
	toolContent := fmt.Sprintf("🔧 Used tool: `%s`", toolCall.Function.Name)
	return su.addContentInternal(ctx, toolContent, updaterStateTool)
}

// AddToolResult records a tool execution result
func (su *SlackUpdater) AddToolResult(ctx context.Context, toolCall api.ToolCall, result []api.Message, err error) error {
	var resultContent string
	if err != nil {
		resultContent = fmt.Sprintf("❌ Tool `%s` failed: %v", toolCall.Function.Name, err)
	} else {
		resultContent = fmt.Sprintf("✅ Tool `%s` completed", toolCall.Function.Name)
		for _, msg := range result {
			if msg.Content != "" {
				resultContent += fmt.Sprintf("\n• %s", msg.Content)
			}
		}
	}
	return su.addContentInternal(ctx, resultContent, updaterStateTool)
}

// UpdateStats handles statistics updates (no-op for Slack UI)
func (su *SlackUpdater) UpdateStats(ctx context.Context, stats query.ConversationStats) error {
	// Slack UI doesn't currently display statistics
	// This provides a hook for future statistics display if needed
	return nil
}

// ForceUpdate provides compatibility with existing code - posts current buffer
func (su *SlackUpdater) ForceUpdate(ctx context.Context) error {
	su.mutex.Lock()
	defer su.mutex.Unlock()

	// Post any remaining buffer content
	_, err := su.switchToType(ctx, updaterStateComplete)
	return err
}

// Flush implements the StreamingUpdater interface
func (su *SlackUpdater) Flush(ctx context.Context) error {
	return su.ForceUpdate(ctx)
}

// getBufferContent returns current buffer content for debugging
func (su *SlackUpdater) getBufferContent() (string, string) {
	su.mutex.Lock()
	defer su.mutex.Unlock()

	var thinkingContent, contentContent string
	currentContent := su.buffer.String()

	if su.currentState == updaterStateThinking {
		thinkingContent = currentContent
	} else {
		contentContent = currentContent
	}

	return contentContent, thinkingContent
}

type oldFormatter struct {
}

func (o *oldFormatter) Format(ctx context.Context, content string, contentType ContentType) ([]slack.Block, error) {
	if contentType == ContentIgnore {
		return nil, nil
	}

	p := NewSlackParser()
	return p.ParseMessageToBlocks(content), nil
}
