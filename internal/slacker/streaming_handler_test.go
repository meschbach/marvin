package slacker

import (
	"testing"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturingUserInterface struct {
	thoughts  []string
	content   []string
	toolCalls []string
}

func (c *capturingUserInterface) AddThought(thought string) {
	c.thoughts = append(c.thoughts, thought)
}
func (c *capturingUserInterface) AddContent(message string) {
	c.content = append(c.content, message)
}
func (c *capturingUserInterface) AddToolCall(message string) {
	c.toolCalls = append(c.toolCalls, message)
}

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
			mock := &capturingUserInterface{}
			handler := newStreamingResponseHandler(mock)
			err := handler.handleResponse(tt.response)
			handler.finished()
			require.NoError(t, err)
			if assert.Len(t, mock.content, len(tt.content), "Expected %#v\n\tGot %#v", tt.content, mock.content) {

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
			mock := &capturingUserInterface{}
			handler := newStreamingResponseHandler(mock)
			err := handler.handleResponse(tt.response)
			require.NoError(t, err)
			handler.finished()
			if assert.Len(t, mock.thoughts, len(tt.thinking), "Expected %#v\n\tGot %#v", tt.thinking, mock.thoughts) {
				for i, expected := range tt.thinking {
					assert.Equal(t, expected, mock.thoughts[i], "Expected @ %d %#v\n\tGot %#v", i, expected, mock.thoughts[i])
				}
			}
		})
	}
}

func TestStreamingResponseHandler_HandleToolCalls(t *testing.T) {
	handler := newStreamingResponseHandler(&SlackUpdater{})

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

	err := handler.handleResponse(response)
	require.NoError(t, err)

	// Test that tool calls are captured in final state
	_, _, calls := handler.finished()
	assert.Len(t, calls, 2)
	assert.Equal(t, "test_tool", calls[0].Function.Name)
	assert.Equal(t, "another_tool", calls[1].Function.Name)
}

func TestStreamingResponseHandler_GetFinalState(t *testing.T) {
	handler := newStreamingResponseHandler(&SlackUpdater{})

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

	for _, resp := range responses {
		err := handler.handleResponse(resp)
		require.NoError(t, err)
	}

	content, thinking, calls := handler.finished()

	assert.Equal(t, "Hello world\n", content)
	assert.Equal(t, "Starting analysis\n", thinking)
	assert.Len(t, calls, 1)
	assert.Equal(t, "test_tool", calls[0].Function.Name)
}

func TestStreamingResponseHandler_DoneResponse(t *testing.T) {
	handler := newStreamingResponseHandler(&SlackUpdater{})

	response := api.ChatResponse{
		Done: true,
	}

	err := handler.handleResponse(response)
	require.NoError(t, err)

	// Should not panic or error on done response
	content, thinking, calls := handler.finished()
	assert.Empty(t, content)
	assert.Empty(t, thinking)
	assert.Empty(t, calls)
}

func TestStreamingResponseHandler_MixedResponse(t *testing.T) {
	handler := newStreamingResponseHandler(&SlackUpdater{})

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

	err := handler.handleResponse(response)
	require.NoError(t, err)

	// Test that all aspects are captured in final state
	content, thinking, calls := handler.finished()
	assert.Equal(t, "Processing data", content)
	assert.Equal(t, "Analyzing input", thinking)
	assert.Len(t, calls, 1)
	assert.Equal(t, "process_data", calls[0].Function.Name)
}

func TestStreamingResponseHandler_BufferAccumulation(t *testing.T) {
	handler := newStreamingResponseHandler(&SlackUpdater{})

	// Test buffer accumulation across multiple responses
	responses := []api.ChatResponse{
		{Message: api.Message{Content: "Part 1 "}},
		{Message: api.Message{Content: "Part 2"}},
		{Message: api.Message{Content: " Part 3\n"}},
	}

	for _, resp := range responses {
		err := handler.handleResponse(resp)
		require.NoError(t, err)
	}

	content, _, _ := handler.finished()
	assert.Equal(t, "Part 1 Part 2 Part 3\n", content)
}

func TestStreamingResponseHandler_ErrorHandling(t *testing.T) {
	// Test that handler doesn't panic on malformed responses
	handler := newStreamingResponseHandler(&SlackUpdater{})

	// Empty response should not error
	response := api.ChatResponse{}
	err := handler.handleResponse(response)
	require.NoError(t, err)
}
