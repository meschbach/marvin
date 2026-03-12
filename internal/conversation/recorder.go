package conversation

import (
	"context"
	"fmt"

	"github.com/ollama/ollama/api"
)

// RecordingUpdater captures conversation stream output for later retrieval.
type RecordingUpdater struct {
	content     string
	thoughts    []string
	toolCalls   []string
	toolResults []string
}

// NewRecordingUpdater creates a new recording updater.
func NewRecordingUpdater() *RecordingUpdater {
	return &RecordingUpdater{
		thoughts:    make([]string, 0),
		toolCalls:   make([]string, 0),
		toolResults: make([]string, 0),
	}
}

// Content returns the accumulated content.
func (ru *RecordingUpdater) Content() string {
	return ru.content
}

// Thoughts returns the captured thoughts.
func (ru *RecordingUpdater) Thoughts() []string {
	return ru.thoughts
}

// ToolCalls returns the captured tool call names.
func (ru *RecordingUpdater) ToolCalls() []string {
	return ru.toolCalls
}

// ToolResults returns the captured tool results.
func (ru *RecordingUpdater) ToolResults() []string {
	return ru.toolResults
}

// AddContent implements StreamingUpdater.
func (ru *RecordingUpdater) AddContent(_ context.Context, content string) error {
	ru.content += content
	return nil
}

// AddThought implements StreamingUpdater.
func (ru *RecordingUpdater) AddThought(_ context.Context, thought string) error {
	ru.thoughts = append(ru.thoughts, thought)
	return nil
}

// AddToolCall implements StreamingUpdater.
func (ru *RecordingUpdater) AddToolCall(_ context.Context, toolCall api.ToolCall) error {
	ru.toolCalls = append(ru.toolCalls, toolCall.Function.Name)
	return nil
}

// AddToolResult implements StreamingUpdater.
func (ru *RecordingUpdater) AddToolResult(_ context.Context, toolCall api.ToolCall, result []api.Message, err error) error {
	if err != nil {
		ru.toolResults = append(ru.toolResults, fmt.Sprintf("%s: error: %v", toolCall.Function.Name, err))
	} else {
		for i := range result {
			ru.toolResults = append(ru.toolResults, fmt.Sprintf("%s: %s", toolCall.Function.Name, result[i].Content))
		}
	}
	return nil
}

// UpdateStats implements StreamingUpdater (no-op for recorder).
func (ru *RecordingUpdater) UpdateStats(_ context.Context, _ Stats) error {
	return nil
}

// Flush implements StreamingUpdater (no-op for recorder).
func (ru *RecordingUpdater) Flush(_ context.Context) error {
	return nil
}
