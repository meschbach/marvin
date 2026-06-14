package openrouter

import (
	"encoding/json"
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/meschbach/marvin/internal/conversation"
	"github.com/meschbach/marvin/internal/llm"
	openrouter2 "github.com/revrost/go-openrouter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenRouterLLM_ConvertMessage_EmptyUserMessageUnchanged(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	msg := llm.Message{
		Role:    conversation.RoleUser,
		Content: "",
	}

	converted := testLLM.convertMessage(t.Context(), msg)

	assert.Equal(t, "", converted.Content.Text, "empty user message should remain empty")
}

func TestOpenRouterLLM_ConvertMessage_WithToolCalls_PreservesID(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	args := map[string]any{"key": "value"}
	argsBytes, _ := json.Marshal(args)

	msg := llm.Message{
		Role: conversation.RoleAssistant,
		ToolCalls: []llm.ToolCall{
			{
				ID: "call_123",
				Function: llm.ToolCallFunction{
					Name:      "test_tool",
					Arguments: args,
				},
			},
		},
	}

	converted := testLLM.convertMessage(t.Context(), msg)

	require.Len(t, converted.ToolCalls, 1)
	assert.Equal(t, "call_123", converted.ToolCalls[0].ID)
	assert.Equal(t, openrouter2.ToolTypeFunction, converted.ToolCalls[0].Type)
	assert.Equal(t, "test_tool", converted.ToolCalls[0].Function.Name)
	assert.JSONEq(t, string(argsBytes), converted.ToolCalls[0].Function.Arguments)
}

func TestOpenRouterLLM_ConvertMessage_WithToolCalls_GeneratesMissingID(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	args := map[string]any{"param": "test"}
	msg := llm.Message{
		Role: conversation.RoleAssistant,
		ToolCalls: []llm.ToolCall{
			{
				ID: "", // empty ID
				Function: llm.ToolCallFunction{
					Name:      "tool_name",
					Arguments: args,
				},
			},
		},
	}

	converted := testLLM.convertMessage(t.Context(), msg)

	require.Len(t, converted.ToolCalls, 1)
	assert.NotEmpty(t, converted.ToolCalls[0].ID, "generated ID should not be empty")
	assert.Equal(t, "call_", converted.ToolCalls[0].ID[:5], "generated ID should start with 'call_'")
	assert.Equal(t, openrouter2.ToolTypeFunction, converted.ToolCalls[0].Type)
	assert.Equal(t, "tool_name", converted.ToolCalls[0].Function.Name)
}

func TestOpenRouterLLM_ConvertToolCallsFromOllama_PreservesArguments(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	// Create tool call with complex arguments
	args := map[string]any{
		"string_param": "hello",
		"number_param": 42,
		"bool_param":   true,
		"array_param":  []interface{}{1, 2, 3},
		"object_param": map[string]interface{}{
			"nested": "value",
		},
	}

	ollamaToolCalls := []llm.ToolCall{
		{
			ID: "call_original",
			Function: llm.ToolCallFunction{
				Name:      "complex_tool",
				Arguments: args,
			},
		},
	}

	// Convert to openrouter format
	openrouterCalls := testLLM.convertToolCallsFromOllama(t.Context(), ollamaToolCalls)

	// Verify conversion
	require.Len(t, openrouterCalls, 1)
	assert.Equal(t, "call_original", openrouterCalls[0].ID)
	assert.Equal(t, "complex_tool", openrouterCalls[0].Function.Name)

	// Verify arguments are correctly serialized to JSON and can be parsed back
	var resultArgs map[string]interface{}
	err := json.Unmarshal([]byte(openrouterCalls[0].Function.Arguments), &resultArgs)
	require.NoError(t, err, "arguments should be valid JSON")

	// Verify the arguments contain expected values
	assert.Equal(t, "hello", resultArgs["string_param"])
	assert.Equal(t, float64(42), resultArgs["number_param"])
	assert.Equal(t, true, resultArgs["bool_param"])
}

func TestOpenRouterLLM_ConvertToolCallsFromOllama_RoundTrip(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	// Original arguments
	originalArgs := map[string]any{
		"query": "test query",
		"limit": 10,
	}

	// Create original tool call
	functionName := faker.Name()
	original := []llm.ToolCall{
		{
			ID: "call_roundtrip",
			Function: llm.ToolCallFunction{
				Name:      functionName,
				Arguments: originalArgs,
			},
		},
	}

	// Convert to openrouter and back
	openrouterCalls := testLLM.convertToolCallsFromOllama(t.Context(), original)
	convertedBack := testLLM.convertToolCallsFromOpenRouter(t.Context(), openrouterCalls)

	//
	require.Len(t, openrouterCalls, 1)
	assert.Equal(t, functionName, openrouterCalls[0].Function.Name)

	// Verify round-trip preserves data
	require.Len(t, convertedBack, 1)
	assert.Equal(t, "call_roundtrip", convertedBack[0].ID)
	assert.Equal(t, functionName, convertedBack[0].Function.Name)

	// Compare arguments - JSON roundtrip converts int to float64 which is expected behavior
	convertedMap := convertedBack[0].Function.Arguments.(map[string]any)

	// Check that the key values exist and have equivalent string representations
	assert.Equal(t, "test query", convertedMap["query"])
	// JSON unmarshal converts numbers to float64
	assert.Equal(t, float64(10), convertedMap["limit"])
}

func TestOpenRouterLLM_ConvertToolCallsFromOllama_MultipleToolCalls(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	// Create multiple tool calls with different arguments
	args1 := map[string]any{"param1": "value1"}
	args2 := map[string]any{"param2": "value2"}

	ollamaToolCalls := []llm.ToolCall{
		{
			ID: "call_1",
			Function: llm.ToolCallFunction{
				Name:      "tool_1",
				Arguments: args1,
			},
		},
		{
			ID: "", // empty ID should be generated
			Function: llm.ToolCallFunction{
				Name:      "tool_2",
				Arguments: args2,
			},
		},
	}

	openrouterCalls := testLLM.convertToolCallsFromOllama(t.Context(), ollamaToolCalls)

	require.Len(t, openrouterCalls, 2)
	assert.Equal(t, "call_1", openrouterCalls[0].ID)
	assert.NotEmpty(t, openrouterCalls[1].ID, "second tool call should have generated ID")
	assert.Equal(t, "tool_1", openrouterCalls[0].Function.Name)
	assert.Equal(t, "tool_2", openrouterCalls[1].Function.Name)

	// Verify arguments
	var resultArgs1, resultArgs2 map[string]interface{}
	json.Unmarshal([]byte(openrouterCalls[0].Function.Arguments), &resultArgs1)
	json.Unmarshal([]byte(openrouterCalls[1].Function.Arguments), &resultArgs2)
	assert.Equal(t, "value1", resultArgs1["param1"])
	assert.Equal(t, "value2", resultArgs2["param2"])
}

func TestOpenRouterLLM_ConvertToolCallsFromOpenRouter_Basic(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	// Create openrouter format tool calls
	openrouterCalls := []openrouter2.ToolCall{
		{
			ID:   "call_123",
			Type: openrouter2.ToolTypeFunction,
			Function: openrouter2.FunctionCall{
				Name:      "test_tool",
				Arguments: `{"key": "value"}`,
			},
			Index: intPtr(0),
		},
	}

	ollamaCalls := testLLM.convertToolCallsFromOpenRouter(t.Context(), openrouterCalls)

	require.Len(t, ollamaCalls, 1)
	assert.Equal(t, "call_123", ollamaCalls[0].ID)
	assert.Equal(t, "test_tool", ollamaCalls[0].Function.Name)

	// Verify arguments
	argsMap := ollamaCalls[0].Function.Arguments.(map[string]any)
	assert.Equal(t, "value", argsMap["key"])
}

func TestOpenRouterLLM_ConvertToolCallsFromOpenRouter_EmptyToolName(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	openrouterCalls := []openrouter2.ToolCall{
		{
			ID:   "call_empty",
			Type: openrouter2.ToolTypeFunction,
			Function: openrouter2.FunctionCall{
				Name:      "", // empty name
				Arguments: `{"key": "value"}`,
			},
			Index: intPtr(0),
		},
		{
			ID:   "call_valid",
			Type: openrouter2.ToolTypeFunction,
			Function: openrouter2.FunctionCall{
				Name:      "valid_tool",
				Arguments: `{"key": "value"}`,
			},
			Index: intPtr(1),
		},
	}

	ollamaCalls := testLLM.convertToolCallsFromOpenRouter(t.Context(), openrouterCalls)

	// Should skip the empty tool name, only valid tool remains
	require.Len(t, ollamaCalls, 1)
	assert.Equal(t, "call_valid", ollamaCalls[0].ID)
	assert.Equal(t, "valid_tool", ollamaCalls[0].Function.Name)
}

func TestOpenRouterLLM_ConvertToolCallsFromOpenRouter_MalformedArguments(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	openrouterCalls := []openrouter2.ToolCall{
		{
			ID:   "call_bad",
			Type: openrouter2.ToolTypeFunction,
			Function: openrouter2.FunctionCall{
				Name:      "bad_tool",
				Arguments: `not valid json`,
			},
			Index: intPtr(0),
		},
	}

	ollamaCalls := testLLM.convertToolCallsFromOpenRouter(t.Context(), openrouterCalls)

	require.Len(t, ollamaCalls, 1)
	assert.Equal(t, "call_bad", ollamaCalls[0].ID)
	assert.Equal(t, "bad_tool", ollamaCalls[0].Function.Name)
	// Arguments should be empty due to malformed JSON
	argsMap := ollamaCalls[0].Function.Arguments.(map[string]any)
	assert.Empty(t, argsMap)
}

func TestOpenRouterLLM_ConvertToolCallsFromOpenRouter_EmptyArguments(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	openrouterCalls := []openrouter2.ToolCall{
		{
			ID:   "call_empty_args",
			Type: openrouter2.ToolTypeFunction,
			Function: openrouter2.FunctionCall{
				Name:      "tool_with_empty_args",
				Arguments: "",
			},
			Index: intPtr(0),
		},
	}

	ollamaCalls := testLLM.convertToolCallsFromOpenRouter(t.Context(), openrouterCalls)

	require.Len(t, ollamaCalls, 1)
	assert.Equal(t, "call_empty_args", ollamaCalls[0].ID)
	assert.Equal(t, "tool_with_empty_args", ollamaCalls[0].Function.Name)
	// Empty arguments should result in empty map
	argsMap := ollamaCalls[0].Function.Arguments.(map[string]any)
	assert.Empty(t, argsMap)
}

func TestOpenRouterLLM_ConvertToolCallsFromOpenRouter_ComplexArguments(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	complexJSON := `{
		"string_param": "hello",
		"number_param": 42,
		"bool_param": true,
		"array_param": [1, 2, 3],
		"object_param": {"nested": "value"},
		"null_param": null
	}`

	openrouterCalls := []openrouter2.ToolCall{
		{
			ID:   "call_complex",
			Type: openrouter2.ToolTypeFunction,
			Function: openrouter2.FunctionCall{
				Name:      "complex_tool",
				Arguments: complexJSON,
			},
			Index: intPtr(0),
		},
	}

	ollamaCalls := testLLM.convertToolCallsFromOpenRouter(t.Context(), openrouterCalls)

	require.Len(t, ollamaCalls, 1)
	assert.Equal(t, "call_complex", ollamaCalls[0].ID)
	assert.Equal(t, "complex_tool", ollamaCalls[0].Function.Name)

	argsMap := ollamaCalls[0].Function.Arguments.(map[string]any)
	assert.Equal(t, "hello", argsMap["string_param"])
	assert.Equal(t, float64(42), argsMap["number_param"])
	assert.Equal(t, true, argsMap["bool_param"])
	// JSON unmarshal converts numbers to float64
	assert.Equal(t, []interface{}{float64(1), float64(2), float64(3)}, argsMap["array_param"])
	assert.Equal(t, map[string]interface{}{"nested": "value"}, argsMap["object_param"])
	assert.Nil(t, argsMap["null_param"])
}

func TestOpenRouterLLM_ConvertToolCallsFromOpenRouter_UnicodeAndSpecialChars(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	complexJSON := `{
		"unicode_text": "你好世界 🌍",
		"emoji": "🚀 🔧 ⚡",
		"special_chars": "!@#$%^&*()_+-=[]{}|;':\",./<>?\\",
		"newlines": "line1\nline2\r\ttab"
	}`

	openrouterCalls := []openrouter2.ToolCall{
		{
			ID:   "call_unicode",
			Type: openrouter2.ToolTypeFunction,
			Function: openrouter2.FunctionCall{
				Name:      "unicode_tool",
				Arguments: complexJSON,
			},
			Index: intPtr(0),
		},
	}

	ollamaCalls := testLLM.convertToolCallsFromOpenRouter(t.Context(), openrouterCalls)

	require.Len(t, ollamaCalls, 1)
	argsMap := ollamaCalls[0].Function.Arguments.(map[string]any)
	assert.Equal(t, "你好世界 🌍", argsMap["unicode_text"])
	assert.Equal(t, "🚀 🔧 ⚡", argsMap["emoji"])
	assert.Equal(t, "!@#$%^&*()_+-=[]{}|;':\",./<>?\\", argsMap["special_chars"])
	// JSON preserves \r characters as literal carriage returns
	assert.Equal(t, "line1\nline2\r\ttab", argsMap["newlines"])
}

func TestOpenRouterLLM_ConvertToolCallsFromOpenRouter_MultipleToolCalls(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	openrouterCalls := []openrouter2.ToolCall{
		{
			ID:   "call_1",
			Type: openrouter2.ToolTypeFunction,
			Function: openrouter2.FunctionCall{
				Name:      "tool_1",
				Arguments: `{"param1": "value1"}`,
			},
			Index: intPtr(0),
		},
		{
			ID:   "call_2",
			Type: openrouter2.ToolTypeFunction,
			Function: openrouter2.FunctionCall{
				Name:      "tool_2",
				Arguments: `{"param2": "value2"}`,
			},
			Index: intPtr(1),
		},
		{
			ID:   "", // empty ID should be preserved (Ollama converts)
			Type: openrouter2.ToolTypeFunction,
			Function: openrouter2.FunctionCall{
				Name:      "tool_3",
				Arguments: `{"param3": "value3"}`,
			},
			Index: intPtr(2),
		},
	}

	ollamaCalls := testLLM.convertToolCallsFromOpenRouter(t.Context(), openrouterCalls)

	require.Len(t, ollamaCalls, 3)
	assert.Equal(t, "call_1", ollamaCalls[0].ID)
	assert.Equal(t, "call_2", ollamaCalls[1].ID)
	assert.Equal(t, "", ollamaCalls[2].ID, "empty ID should be preserved")
	assert.Equal(t, "tool_1", ollamaCalls[0].Function.Name)
	assert.Equal(t, "tool_2", ollamaCalls[1].Function.Name)
	assert.Equal(t, "tool_3", ollamaCalls[2].Function.Name)
}

func TestOpenRouterLLM_ConvertToolCallsFromOpenRouter_PreservesIndex(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	openrouterCalls := []openrouter2.ToolCall{
		{
			ID:   "call_indexed",
			Type: openrouter2.ToolTypeFunction,
			Function: openrouter2.FunctionCall{
				Name:      "indexed_tool",
				Arguments: `{"index": 42}`,
			},
			Index: intPtr(99),
		},
	}

	ollamaCalls := testLLM.convertToolCallsFromOpenRouter(t.Context(), openrouterCalls)

	// Index is part of OpenRouter format but not used in Ollama conversion
	// This test documents that behavior
	require.Len(t, ollamaCalls, 1)
	assert.Equal(t, "call_indexed", ollamaCalls[0].ID)
	assert.Equal(t, "indexed_tool", ollamaCalls[0].Function.Name)
	argsMap := ollamaCalls[0].Function.Arguments.(map[string]any)
	assert.Equal(t, float64(42), argsMap["index"])
}

func TestOpenRouterLLM_ConvertToolCalls_RoundTrip_WithThinkingContext(t *testing.T) {
	testLLM := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	// Simulate a tool call that might appear alongside thinking content
	originalArgs := map[string]any{
		"query": "test query",
		"limit": 10,
	}

	originalOllama := []llm.ToolCall{
		{
			ID: "call_thinking_ctx",
			Function: llm.ToolCallFunction{
				Name:      "search_tool",
				Arguments: originalArgs,
			},
		},
	}

	// Convert Ollama -> OpenRouter (outgoing)
	openrouterCalls := testLLM.convertToolCallsFromOllama(t.Context(), originalOllama)

	require.Len(t, openrouterCalls, 1)
	assert.Equal(t, "search_tool", openrouterCalls[0].Function.Name)

	// Convert OpenRouter -> Ollama (incoming response)
	convertedBack := testLLM.convertToolCallsFromOpenRouter(t.Context(), openrouterCalls)

	// Verify round-trip preserves the tool call (note: ID is preserved from original)
	require.Len(t, convertedBack, 1)
	assert.Equal(t, "call_thinking_ctx", convertedBack[0].ID)
	assert.Equal(t, "search_tool", convertedBack[0].Function.Name)

	convertedMap := convertedBack[0].Function.Arguments.(map[string]any)
	assert.Equal(t, "test query", convertedMap["query"])
	assert.Equal(t, float64(10), convertedMap["limit"])
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}
