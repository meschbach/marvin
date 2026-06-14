package gemini

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/meschbach/marvin/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/genai"
)

func float32Ptr(v float32) *float32 {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func TestConvertOptions(t *testing.T) {
	t.Parallel()
	t.Run("NilOptions_ReturnsEmptyConfig", func(t *testing.T) {
		t.Parallel()
		config, err := convertOptions(nil)
		require.NoError(t, err)
		assert.Nil(t, config.Temperature)
		assert.Nil(t, config.TopP)
		assert.Nil(t, config.TopK)
	})

	t.Run("AllOptions_SetCorrectly", func(t *testing.T) {
		t.Parallel()
		req := &llm.ChatRequest{
			Temperature: float32Ptr(0.5),
			TopP:        float32Ptr(0.8),
			TopK:        intPtr(20),
		}
		config, err := convertOptions(req)
		require.NoError(t, err)

		require.NotNil(t, config.Temperature)
		assert.InEpsilon(t, float32(0.5), *config.Temperature, 0.001)

		require.NotNil(t, config.TopP)
		assert.InEpsilon(t, float32(0.8), *config.TopP, 0.001)

		require.NotNil(t, config.TopK)
		assert.InEpsilon(t, float32(20.0), *config.TopK, 0.001)
	})

	t.Run("PartialOptions_OnlySetsProvided", func(t *testing.T) {
		t.Parallel()
		req := &llm.ChatRequest{
			Temperature: float32Ptr(0.9),
		}
		config, err := convertOptions(req)
		require.NoError(t, err)

		require.NotNil(t, config.Temperature)
		assert.InEpsilon(t, float32(0.9), *config.Temperature, 0.001)

		assert.Nil(t, config.TopP)
		assert.Nil(t, config.TopK)
	})
}

func TestConvertMessages_EmptyMessages(t *testing.T) {
	t.Parallel()
	sys, user, err := convertMessages([]llm.Message{})
	require.NoError(t, err)
	assert.Nil(t, sys)
	assert.Empty(t, user)
}

func TestConvertMessages_UserMessage_PreservesRole(t *testing.T) {
	t.Parallel()
	_, user, err := convertMessages([]llm.Message{{Role: "user", Content: "hello"}})
	require.NoError(t, err)
	require.Len(t, user, 1)
	assert.Equal(t, "user", string(user[0].Role))
	require.NotNil(t, user[0].Parts)
	require.Len(t, user[0].Parts, 1)
	assert.Equal(t, "hello", user[0].Parts[0].Text)
}

func TestConvertMessages_SystemMessage_ExtractedToSeparate(t *testing.T) {
	t.Parallel()
	sys, user, err := convertMessages([]llm.Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "hi"},
	})
	require.NoError(t, err)
	require.NotNil(t, sys)
	require.NotNil(t, sys.Parts)
	require.Len(t, sys.Parts, 1)
	assert.Equal(t, "You are helpful", sys.Parts[0].Text)
	assert.Len(t, user, 1)
	assert.Equal(t, "user", string(user[0].Role))
	require.NotNil(t, user[0].Parts)
	assert.Equal(t, "hi", user[0].Parts[0].Text)
}

func TestConvertMessages_MultipleSystemMessages_UsesLast(t *testing.T) {
	t.Parallel()
	sys, user, err := convertMessages([]llm.Message{
		{Role: "system", Content: "First"},
		{Role: "system", Content: "Second"},
		{Role: "user", Content: "hi"},
	})
	require.NoError(t, err)
	require.NotNil(t, sys)
	require.NotNil(t, sys.Parts)
	assert.Equal(t, "Second", sys.Parts[0].Text)
	assert.Len(t, user, 1)
}

func TestConvertMessages_AssistantMessage_MapsToModel(t *testing.T) {
	t.Parallel()
	_, user, err := convertMessages([]llm.Message{{Role: "assistant", Content: "hi"}})
	require.NoError(t, err)
	require.Len(t, user, 1)
	assert.Equal(t, "model", string(user[0].Role))
}

func TestConvertMessages_ToolRole_MapsToUser(t *testing.T) {
	t.Parallel()
	_, user, err := convertMessages([]llm.Message{{Role: "tool", Content: "result", ToolName: "test_func"}})
	require.NoError(t, err)
	require.Len(t, user, 1)
	assert.Equal(t, "user", string(user[0].Role))
	require.NotNil(t, user[0].Parts[0].FunctionResponse)
	assert.Equal(t, "test_func", user[0].Parts[0].FunctionResponse.Name)
}

func TestConvertMessages_AssistantEmptyContentWithToolCalls_ConvertsToolCalls(t *testing.T) {
	t.Parallel()
	_, user, err := convertMessages([]llm.Message{{
		Role:    "assistant",
		Content: "",
		ToolCalls: []llm.ToolCall{
			{
				ID: "call-1",
				Function: llm.ToolCallFunction{
					Name:      "get_weather",
					Arguments: map[string]any{},
				},
			},
		},
	}})
	require.NoError(t, err)
	require.Len(t, user, 1, "should not skip message when ToolCalls present")
	assert.Equal(t, "model", string(user[0].Role))
	require.NotNil(t, user[0].Parts)
	require.Len(t, user[0].Parts, 1, "should have one part for the tool call")
	assert.NotNil(t, user[0].Parts[0].FunctionResponse, "should be a function response part")
	assert.Equal(t, "get_weather", user[0].Parts[0].FunctionResponse.Name)
}

func TestConvertMessages_AssistantEmptyContentWithMultipleToolCalls_ConvertsAll(t *testing.T) {
	t.Parallel()
	_, user, err := convertMessages([]llm.Message{{
		Role:    "assistant",
		Content: "",
		ToolCalls: []llm.ToolCall{
			{
				ID: "call-1",
				Function: llm.ToolCallFunction{
					Name:      "get_weather",
					Arguments: map[string]any{},
				},
			},
			{
				ID: "call-2",
				Function: llm.ToolCallFunction{
					Name:      "get_time",
					Arguments: map[string]any{},
				},
			},
		},
	}})
	require.NoError(t, err)
	require.Len(t, user, 1)
	require.NotNil(t, user[0].Parts)
	require.Len(t, user[0].Parts, 2, "should have two parts for both tool calls")
	assert.Equal(t, "get_weather", user[0].Parts[0].FunctionResponse.Name)
	assert.Equal(t, "get_time", user[0].Parts[1].FunctionResponse.Name)
}

func TestConvertMessages_AssistantEmptyContentWithoutToolCalls_Skipped(t *testing.T) {
	t.Parallel()
	_, user, err := convertMessages([]llm.Message{{
		Role:    "assistant",
		Content: "",
	}})
	require.NoError(t, err)
	assert.Empty(t, user, "should skip empty assistant message with no tool calls")
}

func TestConvertMessages_UserEmptyContentWithoutToolCalls_Skipped(t *testing.T) {
	t.Parallel()
	_, user, err := convertMessages([]llm.Message{{
		Role:    "user",
		Content: "",
	}})
	require.NoError(t, err)
	assert.Empty(t, user, "should skip empty user message")
}

func TestConvertToLLMResponse(t *testing.T) {
	t.Parallel()
	t.Run("BasicTextResponse", func(t *testing.T) {
		t.Parallel()
		resp := &genai.GenerateContentResponse{
			ModelVersion: "gemini-2.0-flash",
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "Hello world"}},
				},
			}},
		}
		result, err := convertToLLMResponse(resp)
		require.NoError(t, err)

		assert.Equal(t, "Hello world", result.Content)
		assert.False(t, result.Done)
	})

	t.Run("EmptyCandidates", func(t *testing.T) {
		t.Parallel()
		resp := &genai.GenerateContentResponse{
			ModelVersion: "model",
			Candidates:   []*genai.Candidate{},
		}
		result, err := convertToLLMResponse(resp)
		require.NoError(t, err)
		assert.Empty(t, result.Content)
	})

	t.Run("EmptyParts", func(t *testing.T) {
		t.Parallel()
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{}},
			}},
		}
		result, err := convertToLLMResponse(resp)
		require.NoError(t, err)
		assert.Empty(t, result.Content)
	})

	t.Run("EmptyText", func(t *testing.T) {
		t.Parallel()
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{{Text: ""}}},
			}},
		}
		result, err := convertToLLMResponse(resp)
		require.NoError(t, err)
		assert.Empty(t, result.Content)
	})

	t.Run("NilPart", func(t *testing.T) {
		t.Parallel()
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{nil}},
			}},
		}
		result, err := convertToLLMResponse(resp)
		require.NoError(t, err)
		assert.Empty(t, result.Content)
	})

	t.Run("NilContent", func(t *testing.T) {
		t.Parallel()
		resp := &genai.GenerateContentResponse{
			ModelVersion: "model",
			Candidates: []*genai.Candidate{{
				Content: nil,
			}},
		}
		result, err := convertToLLMResponse(resp)
		require.NoError(t, err)
		assert.Empty(t, result.Content)
	})

	t.Run("NilCandidates", func(t *testing.T) {
		t.Parallel()
		resp := &genai.GenerateContentResponse{
			ModelVersion: "model",
			Candidates:   nil,
		}
		result, err := convertToLLMResponse(resp)
		require.NoError(t, err)
		assert.Empty(t, result.Content)
	})

	t.Run("UsageMetadata", func(t *testing.T) {
		t.Parallel()
		resp := &genai.GenerateContentResponse{
			ModelVersion: "gemini-2.0-flash",
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "Hello"}},
				},
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
				TotalTokenCount:      15,
			},
		}
		result, err := convertToLLMResponse(resp)
		require.NoError(t, err)
		assert.Equal(t, 10, result.Stats.PromptTokens)
		assert.Equal(t, 5, result.Stats.ResponseTokens)
	})

}

type MockStreamer struct {
	Responses []*genai.GenerateContentResponse
	Errors    []error
	Index     int
}

func (m *MockStreamer) GenerateContentStream(
	_ context.Context,
	_ string,
	_ []*genai.Content,
	_ *genai.GenerateContentConfig,
) iter.Seq2[*genai.GenerateContentResponse, error] {
	return func(yield func(*genai.GenerateContentResponse, error) bool) {
		// First, yield any initial errors
		if m.Index < len(m.Errors) && m.Errors[m.Index] != nil {
			if !yield(nil, m.Errors[m.Index]) {
				return
			}
			m.Index++
		}

		// Then yield responses
		for m.Index < len(m.Responses) {
			resp := m.Responses[m.Index]
			var err error
			if m.Index < len(m.Errors) {
				err = m.Errors[m.Index]
			}
			m.Index++
			if !yield(resp, err) {
				return
			}
		}
	}
}

type responseCapture struct {
	responses []llm.ChatResponse
}

func (r *responseCapture) OnChatResponse(_ context.Context, resp *llm.ChatResponse) error {
	r.responses = append(r.responses, *resp)
	return nil
}

type stopAtOne struct {
	responses []llm.ChatResponse
}

func (r *stopAtOne) OnChatResponse(_ context.Context, resp *llm.ChatResponse) error {
	r.responses = append(r.responses, *resp)
	if len(r.responses) >= 1 {
		return errors.New("stop")
	}
	return nil
}

func TestLLM_Chat(t *testing.T) {
	t.Parallel()
	t.Run("SingleResponse_Success", func(t *testing.T) {
		t.Parallel()
		mock := &MockStreamer{
			Responses: []*genai.GenerateContentResponse{{
				ModelVersion: "gemini-2.0-flash",
				Candidates: []*genai.Candidate{{
					Content: &genai.Content{
						Parts: []*genai.Part{{Text: "Hello"}},
					},
				}},
			}},
		}
		testLLM := &LLM{client: mock, model: "gemini-2.0-flash"}

		capture := &responseCapture{}
		err := testLLM.Chat(t.Context(), &llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "Hi"}},
		}, capture.OnChatResponse)
		responses := capture.responses

		require.NoError(t, err)
		require.Len(t, responses, 1)
		assert.Equal(t, "Hello", responses[0].Content)
	})

	t.Run("MultipleStreamingResponses", func(t *testing.T) {
		t.Parallel()
		mock := &MockStreamer{
			Responses: []*genai.GenerateContentResponse{
				{
					ModelVersion: "model",
					Candidates: []*genai.Candidate{{
						Content: &genai.Content{Parts: []*genai.Part{{Text: "Hello"}}},
					}},
				},
				{
					ModelVersion: "model",
					Candidates: []*genai.Candidate{{
						Content: &genai.Content{Parts: []*genai.Part{{Text: " world"}}},
					}},
				},
			},
		}
		testLLM := &LLM{client: mock, model: "gemini-2.0-flash"}

		var content string
		capture := &responseCapture{}
		err := testLLM.Chat(t.Context(), &llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "Hi"}},
		}, capture.OnChatResponse)
		for _, resp := range capture.responses {
			content += resp.Content
		}

		require.NoError(t, err)
		assert.Equal(t, "Hello world", content)
	})

	t.Run("ErrorFromStream_PropagatesError", func(t *testing.T) {
		t.Parallel()
		mock := &MockStreamer{
			Responses: []*genai.GenerateContentResponse{},
			Errors:    []error{errors.New("API error")},
		}
		testLLM := &LLM{client: mock, model: "gemini-2.0-flash"}

		capture := &responseCapture{}
		err := testLLM.Chat(t.Context(), &llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "Hi"}},
		}, capture.OnChatResponse)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "API error")
	})

	t.Run("CallbackError_StopsIteration", func(t *testing.T) {
		t.Parallel()
		mock := &MockStreamer{
			Responses: []*genai.GenerateContentResponse{
				{
					ModelVersion: "model",
					Candidates: []*genai.Candidate{{
						Content: &genai.Content{Parts: []*genai.Part{{Text: "First"}}},
					}},
				},
				{
					ModelVersion: "model",
					Candidates: []*genai.Candidate{{
						Content: &genai.Content{Parts: []*genai.Part{{Text: "Second"}}},
					}},
				},
			},
		}
		testLLM := &LLM{client: mock, model: "gemini-2.0-flash"}

		stop := &stopAtOne{}
		err := testLLM.Chat(t.Context(), &llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "Hi"}},
		}, stop.OnChatResponse)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "stop")
		assert.Len(t, stop.responses, 1)
	})

	t.Run("EmptyResponseStream_CompletesWithoutError", func(t *testing.T) {
		t.Parallel()
		mock := &MockStreamer{
			Responses: []*genai.GenerateContentResponse{},
		}
		testLLM := &LLM{client: mock, model: "gemini-2.0-flash"}

		capture := &responseCapture{}
		err := testLLM.Chat(t.Context(), &llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "Hi"}},
		}, capture.OnChatResponse)

		require.NoError(t, err)
		assert.Empty(t, capture.responses)
	})
}
