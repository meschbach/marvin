package conversation

import (
	"context"
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConversationEngine_ContentBeforeDone(t *testing.T) {
	t.Parallel()
	mockLLM := &OneShotLLM{
		responses: []llm.ChatResponse{
			{
				Content: "Hello",
				Done:    false,
			},
			{
				Content: " world",
				Done:    true,
				Stats: llm.Stats{
					ResponseTokens: 10,
					PromptTokens:   5,
				},
			},
		},
	}

	updater := &TrackingUpdater{}
	cfg := &config.File{
		Model: "test-model",
	}
	engine := NewEngine(
		mockLLM,
		cfg,
		&NullLogger{},
		&ToolSet{},
		[]llm.Message{{Role: "user", Content: "hi"}},
	)

	err := engine.RunConversation(t.Context(), "test-model", updater)
	require.NoError(t, err)

	assert.True(t, updater.lastStats.IsDone, "should be done")
	assert.Equal(t, 10, updater.lastStats.EvalCount)

	contentIdx := -1
	doneIdx := -1
	for i, event := range updater.events {
		// we don't care about the remainder types
		switch event.Kind {
		case TrackerEventContent:
			contentIdx = i
		case TrackerEventFlush:
			doneIdx = i
		case TrackerEventThoughts, TrackerEventToolCall, TrackerEventToolResults, TrackerEventDone, TrackerEventStats:
			// ignore other event types
		}
	}

	assert.GreaterOrEqual(t, contentIdx, 0, "should have content event")
	assert.GreaterOrEqual(t, doneIdx, 0, "should have done event")
	assert.Less(t, contentIdx, doneIdx,
		"content should be received before done. Got events: %v", updater.events)
}

func TestConversationEngine_ContentWithDoneInSameChunk(t *testing.T) {
	t.Parallel()
	mockLLM := &OneShotLLM{
		responses: []llm.ChatResponse{
			{
				Content: "Hi there!",
				Done:    true,
				Stats: llm.Stats{
					ResponseTokens: 8,
					PromptTokens:   4,
				},
			},
		},
	}

	updater := &TrackingUpdater{}
	cfg := &config.File{
		Model: "test-model",
	}
	engine := NewEngine(
		mockLLM,
		cfg,
		&NullLogger{},
		&ToolSet{},
		[]llm.Message{{Role: "user", Content: "hello"}},
	)

	err := engine.RunConversation(t.Context(), "test-model", updater)
	require.NoError(t, err)

	assert.True(t, updater.lastStats.IsDone)
	assert.Equal(t, 8, updater.lastStats.EvalCount)

	contentIdx := -1
	doneIdx := -1
	for i, event := range updater.events {
		if event.Kind == TrackerEventContent {
			contentIdx = i
		}
		if event.Kind == TrackerEventFlush {
			doneIdx = i
		}
	}

	assert.GreaterOrEqual(t, contentIdx, 0, "should have content event")
	assert.GreaterOrEqual(t, doneIdx, 0, "should have done event")
	assert.Less(t, contentIdx, doneIdx,
		"content should be received before done. Got events: %v", updater.events)
}

func TestConversationEngine_ThinkingBeforeDone(t *testing.T) {
	t.Parallel()

	paragraph := faker.Paragraph()
	mockLLM := &OneShotLLM{
		responses: []llm.ChatResponse{
			{
				Content:  "",
				Thinking: paragraph,
				Done:     true,
				Stats: llm.Stats{
					ResponseTokens: 15,
					PromptTokens:   10,
				},
			},
		},
	}

	updater := &TrackingUpdater{}
	cfg := &config.File{
		Model: "test-model",
	}
	engine := NewEngine(
		mockLLM,
		cfg,
		&NullLogger{},
		&ToolSet{},
		[]llm.Message{{Role: "user", Content: "think"}},
	)

	err := engine.RunConversation(t.Context(), "test-model", updater)
	require.NoError(t, err)

	assert.True(t, updater.lastStats.IsDone)

	thoughts := updater.Thoughts()
	if assert.Len(t, thoughts, 1) {
		assert.Equal(t, thoughts[0].Value, paragraph)
	}
	// verify the flush is last
	assert.Equal(t, TrackerEventFlush, updater.Last().Kind)
}

func TestConversationEngine_NilConfig(t *testing.T) {
	t.Parallel()

	mockLLM := &OneShotLLM{
		responses: []llm.ChatResponse{
			{
				Content: "Hello",
				Done:    true,
				Stats: llm.Stats{
					ResponseTokens: 5,
					PromptTokens:   3,
				},
			},
		},
	}

	updater := &TrackingUpdater{}
	engine := NewEngine(
		mockLLM,
		nil,
		&NullLogger{},
		&ToolSet{},
		[]llm.Message{{Role: "user", Content: "hi"}},
	)

	err := engine.RunConversation(t.Context(), "test-model", updater)
	require.NoError(t, err, "engine should handle nil config gracefully")

	assert.True(t, updater.lastStats.IsDone)
	assert.Equal(t, 5, updater.lastStats.EvalCount)
}

type toolTrackingLLM struct {
	responses []llm.ChatResponse
	toolCalls []*llm.ChatRequest
	callCount int
}

func (m *toolTrackingLLM) Chat(ctx context.Context, req *llm.ChatRequest, onResponse func(ctx context.Context, resp *llm.ChatResponse) error) error {
	m.toolCalls = append(m.toolCalls, req)

	if m.callCount >= len(m.responses) {
		return nil
	}

	response := m.responses[m.callCount]
	m.callCount++

	if err := onResponse(ctx, &response); err != nil {
		return err
	}
	return nil
}

func TestConversationEngine_SendsToolsOncePerTurn(t *testing.T) {
	t.Parallel()

	toolSet := NewToolSet()
	ctx := t.Context()
	require.NoError(t, toolSet.RegisterTool(ctx, &mockTool{name: "echo", description: "Echo tool"}))
	require.NoError(t, toolSet.RegisterTool(ctx, &mockTool{name: "echo", description: "Duplicate echo tool"}))
	require.NoError(t, toolSet.RegisterTool(ctx, &mockTool{name: "search", description: "Search tool"}))

	mockLLM := &toolTrackingLLM{
		responses: []llm.ChatResponse{
			{
				Content: "I found results",
				Done:    true,
				Stats: llm.Stats{
					ResponseTokens: 10,
					PromptTokens:   5,
				},
			},
		},
	}

	updater := &TrackingUpdater{}
	cfg := &config.File{
		Model: "test-model",
	}
	engine := NewEngine(
		mockLLM,
		cfg,
		&NullLogger{},
		toolSet,
		[]llm.Message{{Role: "user", Content: "search for things"}},
	)

	err := engine.RunConversation(t.Context(), "test-model", updater)
	require.NoError(t, err)

	require.Len(t, mockLLM.toolCalls, 1, "should have one LLM call")
	firstCallTools := mockLLM.toolCalls[0].Tools

	toolNames := make(map[string]int)
	for _, tool := range firstCallTools {
		toolNames[tool.Function.Name]++
	}
	for name, count := range toolNames {
		assert.Equal(t, 1, count, "tool %s should appear exactly once", name)
	}

	assert.Len(t, firstCallTools, 2, "should have exactly 2 unique tools (echo and search)")
}

// TestEngine_ToolSetNotMutatedDuringConversation verifies that the ToolSet's Defs slice
// is not modified during a multi-turn conversation. This catches bugs where code might
// append to Defs during turn execution, causing tools to be duplicated in subsequent turns.
func TestEngine_ToolSetNotMutatedDuringConversation(t *testing.T) {
	t.Parallel()

	toolSet := NewToolSet()
	ctx := t.Context()

	// Register multiple tools
	require.NoError(t, toolSet.RegisterTool(ctx, &mockTool{name: "tool_a", description: "Tool A"}))
	require.NoError(t, toolSet.RegisterTool(ctx, &mockTool{name: "tool_b", description: "Tool B"}))
	require.NoError(t, toolSet.RegisterTool(ctx, &mockTool{name: "tool_c", description: "Tool C"}))

	// Record the initial tool set state
	initialDefs := make([]llm.ToolDefinition, len(toolSet.Defs))
	copy(initialDefs, toolSet.Defs)
	initialDefsCount := len(initialDefs)

	// Create a mock LLM that supports multi-turn with tool calls
	mockLLM := &toolTrackingLLM{
		responses: []llm.ChatResponse{
			// First turn: calls tool_a
			{
				Content: "I'll call tool_a",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Function: llm.ToolCallFunction{Name: "tool_a"}},
				},
				Done: true,
			},
			// Second turn: calls tool_b
			{
				Content: "Now I'll call tool_b",
				ToolCalls: []llm.ToolCall{
					{ID: "call_2", Function: llm.ToolCallFunction{Name: "tool_b"}},
				},
				Done: true,
			},
			// Third turn: final response
			{
				Content: "Final answer",
				Done:    true,
			},
		},
	}

	updater := &TrackingUpdater{}
	cfg := &config.File{Model: "test-model"}

	// Build engine with the tool set
	engine := NewEngine(
		mockLLM,
		cfg,
		&NullLogger{},
		toolSet,
		[]llm.Message{{Role: "user", Content: "start"}},
	)

	// Run the multi-turn conversation
	err := engine.RunConversation(t.Context(), "test-model", updater)
	require.NoError(t, err)

	// Verify the ToolSet.Defs was not mutated
	require.Len(t, toolSet.Defs, initialDefsCount, "ToolSet.Defs length should remain unchanged")
	for i, def := range toolSet.Defs {
		require.Equal(t, initialDefs[i].Function.Name, def.Function.Name,
			"ToolSet.Defs[%d] should be unchanged", i)
	}
}

// TestEngine_NoCumulativeToolSending verifies that across multiple turns,
// the total number of tools sent equals (num_turns * tools_per_call).
// If the bug exists where tools accumulate per turn, this test will fail.
func TestEngine_NoCumulativeToolSending(t *testing.T) {
	t.Parallel()

	toolSet := NewToolSet()
	ctx := t.Context()

	// Register 3 tools
	require.NoError(t, toolSet.RegisterTool(ctx, &mockTool{name: "alpha", description: "Alpha"}))
	require.NoError(t, toolSet.RegisterTool(ctx, &mockTool{name: "beta", description: "Beta"}))
	require.NoError(t, toolSet.RegisterTool(ctx, &mockTool{name: "gamma", description: "Gamma"}))

	// Create a mock LLM with 5 turns
	mockLLM := &toolTrackingLLM{
		responses: []llm.ChatResponse{
			{
				Content:   "Turn 1",
				ToolCalls: []llm.ToolCall{{ID: "1", Function: llm.ToolCallFunction{Name: "alpha"}}},
				Done:      true,
			},
			{
				Content:   "Turn 2",
				ToolCalls: []llm.ToolCall{{ID: "2", Function: llm.ToolCallFunction{Name: "beta"}}},
				Done:      true,
			},
			{
				Content:   "Turn 3",
				ToolCalls: []llm.ToolCall{{ID: "3", Function: llm.ToolCallFunction{Name: "gamma"}}},
				Done:      true,
			},
			{
				Content:   "Turn 4",
				ToolCalls: []llm.ToolCall{{ID: "4", Function: llm.ToolCallFunction{Name: "alpha"}}},
				Done:      true,
			},
			{
				Content: "Turn 5 - done",
				Done:    true,
			},
		},
	}

	updater := &TrackingUpdater{}
	cfg := &config.File{Model: "test-model"}

	engine := NewEngine(mockLLM, cfg, &NullLogger{}, toolSet, []llm.Message{{Role: "user", Content: "start"}})

	err := engine.RunConversation(t.Context(), "test-model", updater)
	require.NoError(t, err)

	// We expect 5 LLM calls
	require.Len(t, mockLLM.toolCalls, 5, "Should have 5 LLM calls")

	// Each call should have exactly 3 tools, not 3, 6, 9, 12, 15
	// If the bug exists, we'd see: 3, 6, 9, 12, 15
	for turnNum, call := range mockLLM.toolCalls {
		toolCount := len(call.Tools)
		assert.Equal(t, 3, toolCount,
			"Turn %d: Should have exactly 3 tools, not %d. If 3*N, tools are accumulating.",
			turnNum+1, toolCount)
	}
}
