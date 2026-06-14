package conversation

import (
	"context"

	"github.com/meschbach/go-junk-bucket/pkg/fx"
	"github.com/meschbach/marvin/internal/llm"
)

type TrackerEventType int

const (
	TrackerEventContent TrackerEventType = iota
	TrackerEventThoughts
	TrackerEventToolCall
	TrackerEventToolResults
	TrackerEventFlush
	TrackerEventDone
	TrackerEventStats
)

type TrackerEvent struct {
	Kind  TrackerEventType
	Value string
	Err   error
	Stats Stats
}

// TrackingUpdater captures the content of the conversation and the last received stats from an LLM
type TrackingUpdater struct {
	events    []TrackerEvent
	lastStats Stats
}

func (t *TrackingUpdater) AddContent(_ context.Context, content string) error {
	t.events = append(t.events, TrackerEvent{Kind: TrackerEventContent, Value: content})
	return nil
}

func (t *TrackingUpdater) AddThought(_ context.Context, thought string) error {
	t.events = append(t.events, TrackerEvent{Kind: TrackerEventThoughts, Value: thought})
	return nil
}

func (t *TrackingUpdater) AddToolCall(_ context.Context, toolCall llm.ToolCall) error {
	t.events = append(t.events, TrackerEvent{Kind: TrackerEventToolCall, Value: toolCall.Function.Name})
	return nil
}

func (t *TrackingUpdater) AddToolResult(_ context.Context, toolCall llm.ToolCall, _ []llm.Message, err error) error {
	t.events = append(t.events, TrackerEvent{Kind: TrackerEventToolResults, Value: toolCall.Function.Name, Err: err})
	return nil
}

func (t *TrackingUpdater) UpdateStats(_ context.Context, stats Stats) error {
	t.events = append(t.events, TrackerEvent{Kind: TrackerEventStats, Stats: stats})
	t.lastStats = stats
	return nil
}

func (t *TrackingUpdater) Flush(_ context.Context) error {
	t.events = append(t.events, TrackerEvent{Kind: TrackerEventFlush})
	return nil
}

func (t *TrackingUpdater) FilterType(eventType TrackerEventType) []TrackerEvent {
	return fx.Filter(t.events, func(event TrackerEvent) bool {
		return event.Kind == eventType
	})
}

func (t *TrackingUpdater) Thoughts() []TrackerEvent {
	return t.FilterType(TrackerEventThoughts)
}

func (t *TrackingUpdater) Last() TrackerEvent {
	return t.events[len(t.events)-1]
}
