package slacker

import (
	"context"
	"testing"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamingResponseHandler_HandleContent(t *testing.T) {
	tests := []struct {
		name     string
		response api.ChatResponse
		content  []string
		thoughts []string
	}{
		{
			name: "simple content",
			response: api.ChatResponse{
				Message: api.Message{
					Content: "Hello world",
				},
			},
			content: []string{"Hello world"},
		},
		{
			name: "content with newline",
			response: api.ChatResponse{
				Message: api.Message{
					Content: "Hello\nworld",
				},
			},
			content: []string{"Hello\nworld"},
		},
		{
			name: "empty content",
			response: api.ChatResponse{
				Message: api.Message{
					Content: "",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockUserInterface{}
			handler := newStreamingResponseHandler(mock)
			ctx := context.Background()
			err := handler.handleResponse(ctx, tt.response)
			_, _, _, err = handler.finished(ctx)
			require.NoError(t, err)
			if assert.Len(t, mock.Content, len(tt.content), "Expected %#v\n\tGot %#v", tt.content, mock.Content) {

			}
		})
	}
}

func TestStreamingResponseHandler_HandleThinking(t *testing.T) {
	tests := []struct {
		name     string
		response api.ChatResponse
		thinking []string
	}{
		{
			name: "simple thinking",
			response: api.ChatResponse{
				Message: api.Message{
					Thinking: "Let me think about this",
				},
			},
			thinking: []string{"Let me think about this"},
		},
		{
			name: "thinking with newline",
			response: api.ChatResponse{
				Message: api.Message{
					Thinking: "Step 1: Analyze\nStep 2: Process",
				},
			},
			thinking: []string{"Step 1: Analyze\nStep 2: Process"},
		},
		{
			name: "empty thinking",
			response: api.ChatResponse{
				Message: api.Message{
					Thinking: "",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockUserInterface{}
			handler := newStreamingResponseHandler(mock)
			ctx := context.Background()
			err := handler.handleResponse(ctx, tt.response)
			require.NoError(t, err)
			_, _, _, err = handler.finished(ctx)
			require.NoError(t, err)
			if assert.Len(t, mock.Thoughts, len(tt.thinking), "Expected %#v\n\tGot %#v", tt.thinking, mock.Thoughts) {
				for i, expected := range tt.thinking {
					assert.Equal(t, expected, mock.Thoughts[i], "Expected @ %d %#v\n\tGot %#v", i, expected, mock.Thoughts[i])
				}
			}
		})
	}
}

func TestStreamingResponseHandler_HandleToolCalls(t *testing.T) {
	mockClient := &MockSlackSink{}
	handler := newStreamingResponseHandler(NewSlackUpdater(mockClient, "test-channel"))

	response := api.ChatResponse{
		Message: api.Message{
			ToolCalls: []api.ToolCall{
				{
					Function: api.ToolCallFunction{
						Name: "test_tool",
					},
				},
				{
					Function: api.ToolCallFunction{
						Name: "another_tool",
					},
				},
			},
		},
	}

	ctx := context.Background()
	err := handler.handleResponse(ctx, response)
	require.NoError(t, err)

	// Test that tool calls are captured in final state
	_, _, calls, err := handler.finished(ctx)
	require.NoError(t, err)
	assert.Len(t, calls, 2)
	assert.Equal(t, "test_tool", calls[0].Function.Name)
	assert.Equal(t, "another_tool", calls[1].Function.Name)
}

func TestStreamingResponseHandler_GetFinalState(t *testing.T) {
	mockClient := &MockSlackSink{}
	handler := newStreamingResponseHandler(NewSlackUpdater(mockClient, "test-channel"))

	// Simulate streaming responses
	responses := []api.ChatResponse{
		{
			Message: api.Message{
				Content:  "Hello",
				Thinking: "Starting",
			},
		},
		{
			Message: api.Message{
				Content:  " world\n",
				Thinking: " analysis\n",
			},
		},
		{
			Message: api.Message{
				ToolCalls: []api.ToolCall{
					{
						Function: api.ToolCallFunction{
							Name: "test_tool",
						},
					},
				},
			},
		},
	}

	ctx := context.Background()
	for _, resp := range responses {
		err := handler.handleResponse(ctx, resp)
		require.NoError(t, err)
	}

	content, thinking, calls, err := handler.finished(ctx)
	require.NoError(t, err)

	assert.Equal(t, "Hello world\n", content)
	assert.Equal(t, "Starting analysis\n", thinking)
	assert.Len(t, calls, 1)
	assert.Equal(t, "test_tool", calls[0].Function.Name)
}

func TestStreamingResponseHandler_DoneResponse(t *testing.T) {
	mockClient := &MockSlackSink{}
	handler := newStreamingResponseHandler(NewSlackUpdater(mockClient, "test-channel"))

	response := api.ChatResponse{
		Done: true,
	}

	ctx := context.Background()
	err := handler.handleResponse(ctx, response)
	require.NoError(t, err)

	// Should not panic or error on done response
	content, thinking, calls, err := handler.finished(ctx)
	require.NoError(t, err)
	assert.Empty(t, content)
	assert.Empty(t, thinking)
	assert.Empty(t, calls)
}

func TestStreamingResponseHandler_MixedResponse(t *testing.T) {
	mockClient := &MockSlackSink{}
	handler := newStreamingResponseHandler(NewSlackUpdater(mockClient, "test-channel"))

	response := api.ChatResponse{
		Message: api.Message{
			Content:  "Processing data",
			Thinking: "Analyzing input",
			ToolCalls: []api.ToolCall{
				{
					Function: api.ToolCallFunction{
						Name: "process_data",
					},
				},
			},
		},
	}

	ctx := context.Background()
	err := handler.handleResponse(ctx, response)
	require.NoError(t, err)

	// Test that all aspects are captured in final state
	content, thinking, calls, err := handler.finished(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Processing data", content)
	assert.Equal(t, "Analyzing input", thinking)
	assert.Len(t, calls, 1)
	assert.Equal(t, "process_data", calls[0].Function.Name)
}

func TestStreamingResponseHandler_BufferAccumulation(t *testing.T) {
	mockClient := &MockSlackSink{}
	handler := newStreamingResponseHandler(NewSlackUpdater(mockClient, "test-channel"))

	// Test buffer accumulation across multiple responses
	responses := []api.ChatResponse{
		{Message: api.Message{Content: "Part 1 "}},
		{Message: api.Message{Content: "Part 2"}},
		{Message: api.Message{Content: " Part 3\n"}},
	}

	ctx := context.Background()
	for _, resp := range responses {
		err := handler.handleResponse(ctx, resp)
		require.NoError(t, err)
	}

	content, _, _, err := handler.finished(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Part 1 Part 2 Part 3\n", content)
}

func TestStreamingResponseHandler_ErrorHandling(t *testing.T) {
	// Test that handler doesn't panic on malformed responses
	mockClient := &MockSlackSink{}
	handler := newStreamingResponseHandler(NewSlackUpdater(mockClient, "test-channel"))

	// Empty response should not error
	ctx := context.Background()
	response := api.ChatResponse{}
	err := handler.handleResponse(ctx, response)
	require.NoError(t, err)
}

// TestStreamingResponseHandler_StateTransitions tests the state machine transitions
func TestStreamingResponseHandler_StateTransitions(t *testing.T) {

	tests := []struct {
		name           string
		responses      []api.ChatResponse
		expectedStates []streamingState
	}{
		{
			name: "thinking to content transition",
			responses: []api.ChatResponse{
				{Message: api.Message{Thinking: "Let me think"}},
				{Message: api.Message{Content: "Here's the answer"}},
			},
			expectedStates: []streamingState{stateThinking, stateContent},
		},
		{
			name: "content to thinking transition",
			responses: []api.ChatResponse{
				{Message: api.Message{Content: "Starting analysis"}},
				{Message: api.Message{Thinking: "Deeper consideration"}},
			},
			expectedStates: []streamingState{stateContent, stateThinking},
		},
		{
			name: "multiple transitions",
			responses: []api.ChatResponse{
				{Message: api.Message{Thinking: "Initial thought"}},
				{Message: api.Message{Content: "First content"}},
				{Message: api.Message{Thinking: "Second thought"}},
				{Message: api.Message{Content: "Final content"}},
			},
			expectedStates: []streamingState{stateThinking, stateContent, stateThinking, stateContent},
		},
		{
			name: "tool call priority",
			responses: []api.ChatResponse{
				{Message: api.Message{Thinking: "Need tools", Content: "And content", ToolCalls: []api.ToolCall{{ID: "test"}}}},
			},
			expectedStates: []streamingState{stateToolCalls},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockUserInterface{}
			handler := newStreamingResponseHandler(mock)

			ctx := context.Background()
			for i, resp := range tt.responses {
				err := handler.handleResponse(ctx, resp)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStates[i], handler.currentState, "State mismatch at response %d", i)
			}
		})
	}
}
