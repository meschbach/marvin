package slacker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/meschbach/marvin/internal/conversation"
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

// Slack character limits
const (
	MaxSectionTextLength = 3000
	MaxBlocksTotal       = 50
	MaxPlainTextLength   = 4000
)

// truncateText cuts text to specified length with ellipsis if needed
func truncateText(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}

	// Try to truncate at word boundary
	truncated := text[:maxLength-3] // Leave room for "..."

	// Find last space before cutoff
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > 0 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// combineBlocks creates a single block from multiple blocks if there are too many
func combineBlocks(blocks []slack.Block) []slack.Block {
	if len(blocks) <= MaxBlocksTotal {
		return blocks
	}

	var combinedText strings.Builder
	for i, block := range blocks {
		if i < MaxBlocksTotal-1 {
			if section, ok := block.(*slack.SectionBlock); ok && section.Text != nil {
				combinedText.WriteString(section.Text.Text)
				combinedText.WriteString("\n\n")
			}
		}
	}

	combinedContent := truncateText(combinedText.String(), MaxSectionTextLength)
	return []slack.Block{
		slack.NewSectionBlock(&slack.TextBlockObject{
			Type: slack.PlainTextType,
			Text: combinedContent,
		}, nil, nil),
	}
}

// truncateBlockText truncates text in a section block if it exceeds limits
func truncateBlockText(block slack.Block, i int) slack.Block {
	section, ok := block.(*slack.SectionBlock)
	if !ok || section.Text == nil {
		return block
	}

	textObj := section.Text
	var truncatedText string
	if textObj.Type == slack.PlainTextType && len(textObj.Text) > MaxPlainTextLength {
		truncatedText = truncateText(textObj.Text, MaxPlainTextLength)
	} else if len(textObj.Text) > MaxSectionTextLength {
		truncatedText = truncateText(textObj.Text, MaxSectionTextLength)
	} else {
		return block
	}

	return slack.NewSectionBlock(&slack.TextBlockObject{
		Type: textObj.Type,
		Text: truncatedText,
	}, nil, nil)
}

// enforceSlackLimits ensures blocks and content respect Slack's limits
func enforceSlackLimits(blocks []slack.Block, message string) ([]slack.Block, string) {
	blocks = combineBlocks(blocks)

	for i, block := range blocks {
		blocks[i] = truncateBlockText(block, i)
	}

	truncatedMessage := truncateText(message, MaxPlainTextLength)
	return blocks, truncatedMessage
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
	client                  SlackSink
	channelID               string
	messageTS               string
	currentState            updaterState
	buffer                  strings.Builder
	mutex                   sync.Mutex
	lastUpdateTime          time.Time
	lastWritten             string
	formatter               ContentFormatter
	timeProvider            TimeProvider
	formattingErrorNotified bool
	preferences             UserPreferences
}

// NewSlackUpdater creates an updater that provides visibility into AI operations.
func NewSlackUpdater(client SlackSink, channelID string, formatter ContentFormatter, preferences UserPreferences, options ...SlackUpdaterOption) *SlackUpdater {
	su := &SlackUpdater{
		client:                  client,
		channelID:               channelID,
		currentState:            updaterStateInit,
		buffer:                  strings.Builder{},
		mutex:                   sync.Mutex{},
		lastUpdateTime:          time.Time{},
		formatter:               formatter,
		timeProvider:            &DefaultTimeProvider{},
		formattingErrorNotified: false,
		preferences:             preferences,
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

	// Reset notification flag for new conversation (init -> thinking transition)
	if su.currentState == updaterStateInit && newState == updaterStateThinking {
		su.formattingErrorNotified = false
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

	// For thinking content, apply formatting based on user preference
	if targetState == updaterStateThinking {
		switch su.preferences.ThinkingFormat {
		case "markdown":
			content = fmt.Sprintf("## 🤔 Thinking\n%s", content)
		case "collapsed":
			content = fmt.Sprintf("🤔 Thinking: %s", content)
		default: // "plain"
			content = fmt.Sprintf("Thinking: %s", content)
		}
	}

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
//
//nolint:gocyclo
func (su *SlackUpdater) updateMessage(ctx context.Context) error {
	message := su.buffer.String()
	if message == "" {
		return nil
	}

	// Parse message to blocks
	blocks, err := su.formatter.Format(ctx, message, su.currentState.toContentType())
	if err != nil {
		// Log formatting error at info level
		log.Printf("[INFO] Formatting error for user %s: %v", su.channelID, err)

		// Create fallback blocks with optional notification
		var fallbackBlocks []slack.Block

		// Show notification only once per conversation
		if !su.formattingErrorNotified {
			notificationBlock := slack.NewSectionBlock(&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: "⚠️ Formatting simplified",
			}, nil, nil)
			fallbackBlocks = append(fallbackBlocks, notificationBlock)
			su.formattingErrorNotified = true
		}

		// Add content as plain text
		contentBlock := slack.NewSectionBlock(&slack.TextBlockObject{
			Type: slack.PlainTextType,
			Text: message,
		}, nil, nil)
		fallbackBlocks = append(fallbackBlocks, contentBlock)

		blocks = fallbackBlocks
	}

	// Enforce Slack character limits on both blocks and message
	blocks, truncatedMessage := enforceSlackLimits(blocks, message)

	if su.messageTS == "" {
		// Post new message
		_, su.messageTS, err = su.client.PostMessageContext(
			ctx,
			su.channelID,
			slack.MsgOptionBlocks(blocks...),
			slack.MsgOptionText(truncatedMessage, true),
		)
		if err != nil {
			// Handle invalid_blocks errors with fallback to plain text
			if strings.Contains(err.Error(), "invalid_blocks") {
				log.Printf("[INFO] Slack API invalid_blocks error for user %s: %v", su.channelID, err)
				_, ts, fallbackErr := su.client.PostMessageContext(
					ctx,
					su.channelID,
					slack.MsgOptionText(truncatedMessage, true),
				)
				if fallbackErr != nil {
					log.Printf("[INFO] Fallback plain text message also failed for user %s: %v", su.channelID, fallbackErr)
					return fallbackErr
				}
				su.messageTS = ts
				su.lastWritten = message
				return nil // Successfully sent as plain text
			}
			return err // Return other types of errors
		}
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
		if err != nil {
			// Handle invalid_blocks errors with fallback to plain text
			if strings.Contains(err.Error(), "invalid_blocks") {
				log.Printf("[INFO] Slack API invalid_blocks error for user %s: %v", su.channelID, err)
				_, _, _, fallbackErr := su.client.UpdateMessageContext(
					ctx,
					su.channelID,
					su.messageTS,
					slack.MsgOptionText(truncatedMessage, true),
				)
				if fallbackErr != nil {
					log.Printf("[INFO] Fallback plain text update also failed for user %s: %v", su.channelID, fallbackErr)
					return fallbackErr
				}
				su.lastWritten = message
				return nil // Successfully updated as plain text
			}
			return err // Return other types of errors
		}
	}

	return nil
}

// AddContent adds content and transitions to content state if needed
func (su *SlackUpdater) AddContent(ctx context.Context, content string) error {
	return su.addContentInternal(ctx, content, updaterStateContent)
}

// AddThought adds thinking content and transitions to thinking state if needed
func (su *SlackUpdater) AddThought(ctx context.Context, thought string) error {
	// Check user preference - always capture internally for metrics, only display if enabled
	if !su.preferences.ShowThinking {
		return nil // Don't display, but thinking is captured elsewhere for metrics
	}

	return su.addContentInternal(ctx, thought, updaterStateThinking)
}

// AddToolCall records a tool call and treats it as regular content
func (su *SlackUpdater) AddToolCall(ctx context.Context, toolCall api.ToolCall) error {
	// Check user preference - always capture internally for metrics
	if !su.preferences.ShowTools {
		return nil // Don't display, but tool calls are captured elsewhere for metrics
	}

	// Tool calls now treated as regular content with thinking-style formatting
	toolContent := fmt.Sprintf("🔧 Used tool: `%s`", toolCall.Function.Name)
	return su.addContentInternal(ctx, toolContent, updaterStateTool)
}

// AddToolResult records a tool execution result
func (su *SlackUpdater) AddToolResult(ctx context.Context, toolCall api.ToolCall, result []api.Message, err error) error {
	// Check user preference - always capture internally for metrics
	if !su.preferences.ShowTools {
		return nil // Don't display, but tool results are captured elsewhere for metrics
	}

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
func (su *SlackUpdater) UpdateStats(ctx context.Context, stats conversation.ConversationStats) error {
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
