package slacker

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"
)

// SlackUpdater handles rate-limited updates to Slack messages
type SlackUpdater struct {
	client         *slack.Client
	channelID      string
	messageTS      string
	lastUpdate     time.Time
	contentBuffer  strings.Builder
	thoughtBuffer  strings.Builder
	toolCalls      []string
	updateInterval time.Duration
	mutex          sync.Mutex
	complete       bool
}

// SlackUpdaterImpl maintains backward compatibility
type SlackUpdaterImpl = SlackUpdater

// NewSlackUpdater creates a new rate-limited message updater
func NewSlackUpdater(client *slack.Client, channelID, messageTS string) *SlackUpdater {
	return &SlackUpdater{
		client:         client,
		channelID:      channelID,
		messageTS:      messageTS,
		lastUpdate:     time.Now(), // Initialize to now to prevent immediate update
		contentBuffer:  strings.Builder{},
		thoughtBuffer:  strings.Builder{},
		toolCalls:      []string{},
		updateInterval: 1 * time.Second,
	}
}

// AddContent adds regular content to the buffer
func (su *SlackUpdater) AddContent(content string) {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	su.contentBuffer.WriteString(content)
}

// AddThought adds thought content to the buffer with proper formatting
func (su *SlackUpdater) AddThought(thought string) {
	su.mutex.Lock()
	defer su.mutex.Unlock()
	su.thoughtBuffer.WriteString(fmt.Sprintf("> Thought: %s", thought))
}

// AddToolCall records a tool call without exposing details
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

// BuildMessage builds the complete message from buffers
func (su *SlackUpdater) BuildMessage() string {
	su.mutex.Lock()
	defer su.mutex.Unlock()

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

// UpdateMessage updates the Slack message if enough time has passed
func (su *SlackUpdater) UpdateMessage() error {
	su.mutex.Lock()
	defer su.mutex.Unlock()

	// Don't update if not enough time has passed and not complete
	if !su.complete && time.Since(su.lastUpdate) < su.updateInterval {
		return nil
	}

	message := su.BuildMessage()

	// Parse message to blocks
	parser := NewSlackParser()
	blocks := parser.ParseMessageToBlocks(message)

	// Update the message with proper block formatting
	_, _, _, err := su.client.UpdateMessage(
		su.channelID,
		su.messageTS,
		slack.MsgOptionBlocks(blocks...),
	)

	if err == nil {
		su.lastUpdate = time.Now()
	}

	return err
}

// MarkComplete marks the response as complete and forces final update
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

	// Then update message without holding the lock during API call
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
