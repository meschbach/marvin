package gemini

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/genai"
)

func TestConvertFloatOption(t *testing.T) {
	t.Run("Float64Value_SetsPointer", func(t *testing.T) {
		var result float32
		opts := map[string]any{"temperature": 0.7}
		convertFloatOption(opts, "temperature", func(v float32) { result = v })
		assert.Equal(t, float32(0.7), result)
	})

	t.Run("WrongType_Ignores", func(t *testing.T) {
		var called bool
		opts := map[string]any{"temperature": "wrong"}
		convertFloatOption(opts, "temperature", func(v float32) { called = true })
		assert.False(t, called, "should not call setter with wrong type")
	})

	t.Run("MissingKey_Ignores", func(t *testing.T) {
		var called bool
		opts := map[string]any{}
		convertFloatOption(opts, "temperature", func(v float32) { called = true })
		assert.False(t, called)
	})
}

func TestConvertIntToFloatOption(t *testing.T) {
	t.Run("IntValue_ConvertsToFloat32", func(t *testing.T) {
		var result float32
		opts := map[string]any{"top_k": 40}
		convertIntToFloatOption(opts, "top_k", func(v float32) { result = v })
		assert.Equal(t, float32(40.0), result)
	})

	t.Run("WrongTypeFloat_Ignores", func(t *testing.T) {
		var called bool
		opts := map[string]any{"top_k": 40.5}
		convertIntToFloatOption(opts, "top_k", func(v float32) { called = true })
		assert.False(t, called)
	})

	t.Run("WrongTypeString_Ignores", func(t *testing.T) {
		var called bool
		opts := map[string]any{"top_k": "forty"}
		convertIntToFloatOption(opts, "top_k", func(v float32) { called = true })
		assert.False(t, called)
	})
}

func TestConvertIntOption(t *testing.T) {
	t.Run("IntValue_SetsInt32", func(t *testing.T) {
		var result int32
		opts := map[string]any{"num_predict": 100}
		convertIntOption(opts, "num_predict", func(v int32) { result = v })
		assert.Equal(t, int32(100), result)
	})

	t.Run("WrongType_Ignores", func(t *testing.T) {
		var called bool
		opts := map[string]any{"num_predict": "100"}
		convertIntOption(opts, "num_predict", func(v int32) { called = true })
		assert.False(t, called)
	})
}

func TestConvertStopSequences(t *testing.T) {
	t.Run("StringArray_ExtractsAll", func(t *testing.T) {
		opts := map[string]any{"stop": []any{"END", "STOP", "DONE"}}
		config := &genai.GenerateContentConfig{}
		convertStopSequences(opts, config)
		assert.Equal(t, []string{"END", "STOP", "DONE"}, config.StopSequences)
	})

	t.Run("MixedArray_FiltersStrings", func(t *testing.T) {
		opts := map[string]any{"stop": []any{"END", 123, "STOP"}}
		config := &genai.GenerateContentConfig{}
		convertStopSequences(opts, config)
		assert.Equal(t, []string{"END", "STOP"}, config.StopSequences)
	})

	t.Run("WrongType_Ignores", func(t *testing.T) {
		opts := map[string]any{"stop": "not-an-array"}
		config := &genai.GenerateContentConfig{}
		convertStopSequences(opts, config)
		assert.Nil(t, config.StopSequences)
	})

	t.Run("EmptyArray", func(t *testing.T) {
		opts := map[string]any{"stop": []any{}}
		config := &genai.GenerateContentConfig{}
		convertStopSequences(opts, config)
		assert.Empty(t, config.StopSequences)
	})
}

func TestConvertOptions(t *testing.T) {
	t.Run("NilOptions_ReturnsEmptyConfig", func(t *testing.T) {
		config := convertOptions(nil)
		assert.Nil(t, config.Temperature)
		assert.Nil(t, config.TopP)
		assert.Nil(t, config.TopK)
		assert.Equal(t, int32(0), config.MaxOutputTokens)
		assert.Nil(t, config.StopSequences)
	})

	t.Run("AllOptions_SetCorrectly", func(t *testing.T) {
		opts := map[string]any{
			"temperature": 0.5,
			"top_p":       0.8,
			"top_k":       20,
			"num_predict": 50,
			"stop":        []any{"END"},
		}
		config := convertOptions(opts)

		require.NotNil(t, config.Temperature)
		assert.Equal(t, float32(0.5), *config.Temperature)

		require.NotNil(t, config.TopP)
		assert.Equal(t, float32(0.8), *config.TopP)

		require.NotNil(t, config.TopK)
		assert.Equal(t, float32(20.0), *config.TopK)

		assert.Equal(t, int32(50), config.MaxOutputTokens)
		assert.Equal(t, []string{"END"}, config.StopSequences)
	})

	t.Run("PartialOptions_OnlySetsProvided", func(t *testing.T) {
		opts := map[string]any{
			"temperature": 0.9,
		}
		config := convertOptions(opts)

		require.NotNil(t, config.Temperature)
		assert.Equal(t, float32(0.9), *config.Temperature)

		assert.Nil(t, config.TopP)
		assert.Nil(t, config.TopK)
		assert.Equal(t, int32(0), config.MaxOutputTokens)
	})
}

func TestConvertMessages_EmptyMessages(t *testing.T) {
	sys, user, err := convertMessages([]api.Message{})
	require.NoError(t, err)
	assert.Nil(t, sys)
	assert.Empty(t, user)
}

func TestConvertMessages_UserMessage_PreservesRole(t *testing.T) {
	_, user, err := convertMessages([]api.Message{{Role: "user", Content: "hello"}})
	require.NoError(t, err)
	require.Len(t, user, 1)
	assert.Equal(t, "user", string(user[0].Role))
	require.NotNil(t, user[0].Parts)
	require.Len(t, user[0].Parts, 1)
	assert.Equal(t, "hello", user[0].Parts[0].Text)
}

func TestConvertMessages_SystemMessage_ExtractedToSeparate(t *testing.T) {
	sys, user, err := convertMessages([]api.Message{
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
	sys, user, err := convertMessages([]api.Message{
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
	_, user, err := convertMessages([]api.Message{{Role: "assistant", Content: "hi"}})
	require.NoError(t, err)
	require.Len(t, user, 1)
	assert.Equal(t, "model", string(user[0].Role))
}

func TestConvertMessages_ToolRole_MapsToUser(t *testing.T) {
	_, user, err := convertMessages([]api.Message{{Role: "tool", Content: "result"}})
	require.NoError(t, err)
	require.Len(t, user, 1)
	assert.Equal(t, "user", string(user[0].Role))
}

func TestConvertMessages_AssistantEmptyContentWithToolCalls_ConvertsToolCalls(t *testing.T) {
	_, user, err := convertMessages([]api.Message{{
		Role:    "assistant",
		Content: "",
		ToolCalls: []api.ToolCall{
			{
				ID: "call-1",
				Function: api.ToolCallFunction{
					Name:      "get_weather",
					Arguments: api.NewToolCallFunctionArguments(),
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
	_, user, err := convertMessages([]api.Message{{
		Role:    "assistant",
		Content: "",
		ToolCalls: []api.ToolCall{
			{
				ID: "call-1",
				Function: api.ToolCallFunction{
					Name:      "get_weather",
					Arguments: api.NewToolCallFunctionArguments(),
				},
			},
			{
				ID: "call-2",
				Function: api.ToolCallFunction{
					Name:      "get_time",
					Arguments: api.NewToolCallFunctionArguments(),
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
	_, user, err := convertMessages([]api.Message{{
		Role:    "assistant",
		Content: "",
	}})
	require.NoError(t, err)
	assert.Empty(t, user, "should skip empty assistant message with no tool calls")
}

func TestConvertMessages_UserEmptyContentWithoutToolCalls_Skipped(t *testing.T) {
	_, user, err := convertMessages([]api.Message{{
		Role:    "user",
		Content: "",
	}})
	require.NoError(t, err)
	assert.Empty(t, user, "should skip empty user message")
}

//nolint:funlen
func TestConvertToOllamaResponse(t *testing.T) {
	t.Run("BasicTextResponse", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			ModelVersion: "gemini-2.0-flash",
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "Hello world"}},
				},
			}},
		}
		result, err := convertToOllamaResponse(resp)
		require.NoError(t, err)

		assert.Equal(t, "gemini-2.0-flash", result.Model)
		assert.Equal(t, "Hello world", result.Message.Content)
		assert.Equal(t, "assistant", result.Message.Role)
		assert.False(t, result.Done)
	})

	t.Run("EmptyCandidates", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			ModelVersion: "model",
			Candidates:   []*genai.Candidate{},
		}
		result, err := convertToOllamaResponse(resp)
		require.NoError(t, err)
		assert.Equal(t, "model", result.Model)
		assert.Empty(t, result.Message.Content)
	})

	t.Run("EmptyParts", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{}},
			}},
		}
		result, err := convertToOllamaResponse(resp)
		require.NoError(t, err)
		assert.Empty(t, result.Message.Content)
	})

	t.Run("EmptyText", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{{Text: ""}}},
			}},
		}
		result, err := convertToOllamaResponse(resp)
		require.NoError(t, err)
		assert.Empty(t, result.Message.Content)
	})

	t.Run("NilPart", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{nil}},
			}},
		}
		result, err := convertToOllamaResponse(resp)
		require.NoError(t, err)
		assert.Empty(t, result.Message.Content)
	})

	t.Run("NilContent", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			ModelVersion: "model",
			Candidates: []*genai.Candidate{{
				Content: nil,
			}},
		}
		result, err := convertToOllamaResponse(resp)
		require.NoError(t, err)
		assert.Equal(t, "model", result.Model)
		assert.Empty(t, result.Message.Content)
	})

	t.Run("NilCandidates", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			ModelVersion: "model",
			Candidates:   nil,
		}
		result, err := convertToOllamaResponse(resp)
		require.NoError(t, err)
		assert.Equal(t, "model", result.Model)
		assert.Empty(t, result.Message.Content)
	})

	t.Run("UsageMetadata", func(t *testing.T) {
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
		result, err := convertToOllamaResponse(resp)
		require.NoError(t, err)
		assert.Equal(t, 10, result.PromptEvalCount)
		assert.Equal(t, 5, result.EvalCount)
	})

	t.Run("CreatedAtIsSet", func(t *testing.T) {
		before := time.Now()
		resp := &genai.GenerateContentResponse{
			ModelVersion: "model",
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
			}},
		}
		result, err := convertToOllamaResponse(resp)
		require.NoError(t, err)
		after := time.Now()

		assert.True(t, result.CreatedAt.After(before) || result.CreatedAt.Equal(before))
		assert.True(t, result.CreatedAt.Before(after) || result.CreatedAt.Equal(after))
	})
}

type MockStreamer struct {
	Responses []*genai.GenerateContentResponse
	Errors    []error
	Index     int
}

func (m *MockStreamer) GenerateContentStream(
	ctx context.Context,
	model string,
	contents []*genai.Content,
	config *genai.GenerateContentConfig,
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

//nolint:funlen
func TestLLM_Chat(t *testing.T) {
	t.Run("SingleResponse_Success", func(t *testing.T) {
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
		llm := &LLM{client: mock, model: "gemini-2.0-flash"}

		var responses []api.ChatResponse
		err := llm.Chat(context.Background(), &api.ChatRequest{
			Messages: []api.Message{{Role: "user", Content: "Hi"}},
		}, func(resp api.ChatResponse) error {
			responses = append(responses, resp)
			return nil
		})

		require.NoError(t, err)
		require.Len(t, responses, 1)
		assert.Equal(t, "Hello", responses[0].Message.Content)
		assert.Equal(t, "gemini-2.0-flash", responses[0].Model)
	})

	t.Run("MultipleStreamingResponses", func(t *testing.T) {
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
		llm := &LLM{client: mock, model: "gemini-2.0-flash"}

		var content string
		err := llm.Chat(context.Background(), &api.ChatRequest{
			Messages: []api.Message{{Role: "user", Content: "Hi"}},
		}, func(resp api.ChatResponse) error {
			content += resp.Message.Content
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, "Hello world", content)
	})

	t.Run("ErrorFromStream_PropagatesError", func(t *testing.T) {
		mock := &MockStreamer{
			Responses: []*genai.GenerateContentResponse{},
			Errors:    []error{errors.New("API error")},
		}
		llm := &LLM{client: mock, model: "gemini-2.0-flash"}

		err := llm.Chat(context.Background(), &api.ChatRequest{
			Messages: []api.Message{{Role: "user", Content: "Hi"}},
		}, func(resp api.ChatResponse) error {
			return nil
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "API error")
	})

	t.Run("CallbackError_StopsIteration", func(t *testing.T) {
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
		llm := &LLM{client: mock, model: "gemini-2.0-flash"}

		var responses []api.ChatResponse
		err := llm.Chat(context.Background(), &api.ChatRequest{
			Messages: []api.Message{{Role: "user", Content: "Hi"}},
		}, func(resp api.ChatResponse) error {
			responses = append(responses, resp)
			if len(responses) == 1 {
				return errors.New("stop iteration")
			}
			return nil
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "stop iteration")
		assert.Len(t, responses, 1)
	})

	t.Run("EmptyResponseStream_CompletesWithoutError", func(t *testing.T) {
		mock := &MockStreamer{
			Responses: []*genai.GenerateContentResponse{},
		}
		llm := &LLM{client: mock, model: "gemini-2.0-flash"}

		var responses []api.ChatResponse
		err := llm.Chat(context.Background(), &api.ChatRequest{
			Messages: []api.Message{{Role: "user", Content: "Hi"}},
		}, func(resp api.ChatResponse) error {
			responses = append(responses, resp)
			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, responses)
	})
}
