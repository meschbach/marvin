package slacker

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"
)

// SlackSink provides an abstraction for Slack message operations.
// It enables testing by allowing mock implementations and supports different
// Slack client strategies while maintaining a consistent interface for
// real-time message operations.
type SlackSink interface {
	//todo: should use the context version
	UpdateMessage(channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error)
	PostMessage(channelID string, options ...slack.MsgOption) (string, string, error)
}

// SlackUpdater provides real-time progress updates for long-running AI operations in Slack.
// It solves the user experience problem of silent AI processing by showing incremental progress,
// thoughts, and tool usage while preventing API rate limiting through intelligent throttling.
type SlackUpdater struct {
	client         SlackSink
	channelID      string
	messageTS      string
	lastUpdate     time.Time
	contentBuffer  strings.Builder
	thoughtBuffer  strings.Builder
	toolCalls      []string
	updateInterval time.Duration
	mutex          sync.Mutex
	complete       bool
	messagePosted  bool
}

// NewSlackUpdater creates an updater that provides visibility into AI operations.
// It transforms the "black box" experience of AI processing into an interactive session
// where users can see the AI reasoning, tool usage, and progress in real-time.
func NewSlackUpdater(client SlackSink, channelID string) *SlackUpdater {
	return &SlackUpdater{
		client:         client,
		channelID:      channelID,
		lastUpdate:     time.Now(), // Initialize to now to prevent immediate update
		contentBuffer:  strings.Builder{},
		thoughtBuffer:  strings.Builder{},
		toolCalls:      []string{},
		updateInterval: 1 * time.Second,
		mutex:          sync.Mutex{},
		messagePosted:  false,
	}
}

// AddContent adds regular content to the buffer
func (su *SlackUpdater) AddContent(content string) {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	su.contentBuffer.WriteString(content)
}

// AddThought reveals AI reasoning to build trust and transparency.
// Seeing the thought process helps users understand how decisions are made
// and provides insight into the AI's problem-solving approach.
func (su *SlackUpdater) AddThought(thought string) {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	su.thoughtBuffer.WriteString(fmt.Sprintf("> Thought: %s", thought))
}

// AddToolCall provides transparency into tool usage without exposing sensitive data.
// This shows users what actions are being taken on their behalf, building confidence
// in the AI's capabilities while maintaining security boundaries.
func (su *SlackUpdater) AddToolCall(toolName string) {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	su.toolCalls = append(su.toolCalls, toolName)
}

// ShouldUpdate checks if enough time has passed since the last update
func (su *SlackUpdater) ShouldUpdate() bool {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	return time.Since(su.lastUpdate) >= su.updateInterval
}

// buildMessage builds the complete message from buffers
// Note: This method assumes the caller already holds the mutex lock
func (su *SlackUpdater) buildMessage() string {
	var message strings.Builder

	// Add content if present
	if su.contentBuffer.Len() > 0 {
		message.WriteString(su.contentBuffer.String())
	}

	// Add thoughts if present
	if su.thoughtBuffer.Len() > 0 {
		if message.Len() > 0 {
			message.WriteString("\n\n")
		}
		message.WriteString(su.thoughtBuffer.String())
	}

	// Add tool call notifications if present
	if len(su.toolCalls) > 0 {
		if message.Len() > 0 {
			message.WriteString("\n\n")
		}
		message.WriteString("🔧 Tools used: ")
		for i, tool := range su.toolCalls {
			if i > 0 {
				message.WriteString(", ")
			}
			message.WriteString(fmt.Sprintf("`%s`", tool))
		}
	}

	return message.String()
}

// BuildMessage builds the complete message from buffers with proper locking
func (su *SlackUpdater) BuildMessage() string {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	return su.buildMessage()
}

// UpdateMessage posts or updates the Slack message if enough time has passed
func (su *SlackUpdater) UpdateMessage() error {
	su.mutex.Lock()
	defer su.mutex.Unlock()

	// Don't update if not enough time has passed and not complete
	if !su.complete && time.Since(su.lastUpdate) < su.updateInterval {
		return nil
	}

	message := su.buildMessage()

	// Parse message to blocks
	parser := NewSlackParser()
	blocks := parser.ParseMessageToBlocks(message)

	var err error

	if !su.messagePosted {
		// Post the first message
		_, ts, err := su.client.PostMessage(
			su.channelID,
			slack.MsgOptionBlocks(blocks...),
		)
		if err == nil {
			su.messageTS = ts
			su.messagePosted = true
			su.lastUpdate = time.Now()
		}
	} else {
		// Update the existing message
		_, _, _, err = su.client.UpdateMessage(
			su.channelID,
			su.messageTS,
			slack.MsgOptionBlocks(blocks...),
		)
		if err == nil {
			su.lastUpdate = time.Now()
		}
	}

	return err
}

// MarkComplete signals that AI processing has finished, enabling the final update.
// This ensures users get a complete, final message rather than being left with
// a partially displayed response.
func (su *SlackUpdater) MarkComplete() {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	su.complete = true
}

// ForceUpdate forces an immediate update regardless of timing
func (su *SlackUpdater) ForceUpdate() error {
	// Mark as complete first
	su.mutex.Lock()
	su.complete = true
	su.mutex.Unlock()

	// Then update message - UpdateMessage handles its own locking
	if err := su.UpdateMessage(); err != nil {
		return err
	}
	return nil
}

// GetBufferContent returns current buffer content for debugging
func (su *SlackUpdater) GetBufferContent() (string, string, []string) {
	su.mutex.Lock()
	defer su.mutex.Unlock()

	return su.contentBuffer.String(), su.thoughtBuffer.String(), su.toolCalls
}
