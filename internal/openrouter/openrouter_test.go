package openrouter

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/meschbach/marvin/internal/conversation"
	"github.com/ollama/ollama/api"
	"github.com/revrost/go-openrouter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTransport struct {
	respBody string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(m.respBody)),
		Header:     http.Header{},
	}, nil
}

func TestOpenRouterLLM_Chat_StreamsContentAndMetrics(t *testing.T) {
	respBody := `data: {"id":"gen-123","model":"openai/gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":null}

data: {"id":"gen-123","model":"openai/gpt-4o-mini","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
`

	mockHTTPClient := &http.Client{Transport: &mockTransport{respBody: respBody}}

	config := openrouter.DefaultConfig("test-key")
	config.BaseURL = "https://openrouter.ai/api/v1"
	config.HTTPClient = mockHTTPClient

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "openai/gpt-4o-mini",
		httpClient: openrouter.NewClientWithConfig(*config),
	}

	req := &api.ChatRequest{
		Messages: []api.Message{
			{Role: "user", Content: "Hi"},
		},
	}

	var responses []api.ChatResponse
	err := llm.Chat(context.Background(), req, func(resp api.ChatResponse) error {
		responses = append(responses, resp)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, responses, 2, "expected 2 responses: one content, one done")

	first := responses[0]
	assert.Equal(t, "Hello", first.Message.Content)
	assert.False(t, first.Done, "first response should not be done yet")
	assert.Equal(t, 0, first.EvalCount, "first response should have no metrics yet")

	second := responses[1]
	assert.True(t, second.Done, "second response should be done")
	assert.Equal(t, 5, second.EvalCount, "completion tokens should be 5")
	assert.Equal(t, 10, second.PromptEvalCount, "prompt tokens should be 10")
}

func TestOpenRouterLLM_Chat_StreamsContentAndMetrics_WithChunkedContent(t *testing.T) {
	respBody := `data: {"id":"gen-123","model":"openai/gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":null}

data: {"id":"gen-123","model":"openai/gpt-4o-mini","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
`

	mockHTTPClient := &http.Client{Transport: &mockTransport{respBody: respBody}}

	config := openrouter.DefaultConfig("test-key")
	config.BaseURL = "https://openrouter.ai/api/v1"
	config.HTTPClient = mockHTTPClient

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "openai/gpt-4o-mini",
		httpClient: openrouter.NewClientWithConfig(*config),
	}

	req := &api.ChatRequest{
		Messages: []api.Message{
			{Role: "user", Content: "Hi"},
		},
	}

	var lastResponse api.ChatResponse
	err := llm.Chat(context.Background(), req, func(resp api.ChatResponse) error {
		lastResponse = resp
		return nil
	})

	require.NoError(t, err)
	assert.True(t, lastResponse.Done, "last response should be done")
	assert.Equal(t, 5, lastResponse.EvalCount, "completion tokens should be 5")
	assert.Equal(t, 10, lastResponse.PromptEvalCount, "prompt tokens should be 10")
}

func TestOpenRouterLLM_Chat_UsageInSameChunkAsFinishReason(t *testing.T) {
	respBody := `data: {"id":"gen-123","model":"openai/gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
`

	mockHTTPClient := &http.Client{Transport: &mockTransport{respBody: respBody}}

	config := openrouter.DefaultConfig("test-key")
	config.BaseURL = "https://openrouter.ai/api/v1"
	config.HTTPClient = mockHTTPClient

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "openai/gpt-4o-mini",
		httpClient: openrouter.NewClientWithConfig(*config),
	}

	req := &api.ChatRequest{
		Messages: []api.Message{
			{Role: "user", Content: "Hi"},
		},
	}

	var lastResponse api.ChatResponse
	err := llm.Chat(context.Background(), req, func(resp api.ChatResponse) error {
		lastResponse = resp
		return nil
	})

	require.NoError(t, err)
	assert.True(t, lastResponse.Done, "response should be done")
	assert.Equal(t, 5, lastResponse.EvalCount, "completion tokens should be 5")
	assert.Equal(t, 10, lastResponse.PromptEvalCount, "prompt tokens should be 10")
}

func TestOpenRouterLLM_Chat_NemotronRealResponseFormat(t *testing.T) {
	respBody := `data: {"id":"gen-1770951369-sKYcTnzlyj6HvxZfXXkd","provider":"Nvidia","model":"nvidia/nemotron-3-nano-30b-a3b:free","object":"chat.completion.chunk","created":1770951369,"choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"gen-1770951369-sKYcTnzlyj6HvxZfXXkd","provider":"Nvidia","model":"nvidia/nemotron-3-nano-30b-a3b:free","object":"chat.completion.chunk","created":1770951369,"choices":[{"index":0,"delta":{"role":"assistant","content":"Hi"},"finish_reason":null}]}

data: {"id":"gen-1770951369-sKYcTnzlyj6HvxZfXXkd","provider":"Nvidia","model":"nvidia/nemotron-3-nano-30b-a3b:free","object":"chat.completion.chunk","created":1770951369,"choices":[{"index":0,"delta":{"role":"assistant","content":" there"},"finish_reason":null}]}

data: {"id":"gen-1770951369-sKYcTnzlyj6HvxZfXXkd","provider":"Nvidia","model":"nvidia/nemotron-3-nano-30b-a3b:free","object":"chat.completion.chunk","created":1770951369,"choices":[{"index":0,"delta":{"role":"assistant","content":"!"},"finish_reason":null}]}

data: {"id":"gen-1770951369-sKYcTnzlyj6HvxZfXXkd","provider":"Nvidia","model":"nvidia/nemotron-3-nano-30b-a3b:free","object":"chat.completion.chunk","created":1770951369,"choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":"stop"}]}

data: {"id":"gen-1770951369-sKYcTnzlyj6HvxZfXXkd","provider":"Nvidia","model":"nvidia/nemotron-3-nano-30b-a3b:free","object":"chat.completion.chunk","created":1770951369,"choices":[{"index":0,"delta":{"role":"assistant","content":""}}],"usage":{"prompt_tokens":18,"completion_tokens":47,"total_tokens":65}}

data: [DONE]
`

	mockHTTPClient := &http.Client{Transport: &mockTransport{respBody: respBody}}

	config := openrouter.DefaultConfig("test-key")
	config.BaseURL = "https://openrouter.ai/api/v1"
	config.HTTPClient = mockHTTPClient

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "nvidia/nemotron-3-nano-30b-a3b:free",
		httpClient: openrouter.NewClientWithConfig(*config),
	}

	req := &api.ChatRequest{
		Messages: []api.Message{
			{Role: "user", Content: "Say hi"},
		},
	}

	var responses []api.ChatResponse
	err := llm.Chat(context.Background(), req, func(resp api.ChatResponse) error {
		responses = append(responses, resp)
		return nil
	})

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(responses), 1, "should have at least one response")

	lastResponse := responses[len(responses)-1]
	assert.True(t, lastResponse.Done, "last response should be done")
	assert.Equal(t, 47, lastResponse.EvalCount, "completion tokens should be 47")
	assert.Equal(t, 18, lastResponse.PromptEvalCount, "prompt tokens should be 18")
	assert.Equal(t, "", lastResponse.Message.Content, "last chunk should have empty content (engine accumulates)")
}

func TestOpenRouterLLM_Chat_ToolCallWithEmptyArguments(t *testing.T) {
	respBody := "data: {\"id\":\"gen-123\",\"model\":\"openai/gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"id\":\"call_empty\",\"type\":\"function\",\"function\":{\"name\":\"noop\",\"arguments\":\"\"}}]},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":5,\"total_tokens\":10}}\n\n" +
		"data: [DONE]\n"

	mockHTTPClient := &http.Client{Transport: &mockTransport{respBody: respBody}}

	config := openrouter.DefaultConfig("test-key")
	config.BaseURL = "https://openrouter.ai/api/v1"
	config.HTTPClient = mockHTTPClient

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "openai/gpt-4o-mini",
		httpClient: openrouter.NewClientWithConfig(*config),
	}

	req := &api.ChatRequest{
		Messages: []api.Message{
			{Role: "user", Content: "Do nothing"},
		},
	}

	var toolCallResp *api.ChatResponse
	err := llm.Chat(context.Background(), req, func(resp api.ChatResponse) error {
		if len(resp.Message.ToolCalls) > 0 {
			toolCallResp = &resp
		}
		return nil
	})

	require.NoError(t, err)
	require.NotNil(t, toolCallResp, "should receive a response with tool calls")
	require.Len(t, toolCallResp.Message.ToolCalls, 1, "tool call should be preserved even with empty arguments")

	assert.Equal(t, "call_empty", toolCallResp.Message.ToolCalls[0].ID, "ID should be preserved")
	assert.Equal(t, "noop", toolCallResp.Message.ToolCalls[0].Function.Name, "Name should NOT be empty - this is the bug!")
}

func TestOpenRouterLLM_Chat_ToolCallWithMalformedArguments(t *testing.T) {
	respBody := "data: {\"id\":\"gen-123\",\"model\":\"openai/gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"id\":\"call_bad\",\"type\":\"function\",\"function\":{\"name\":\"bad_tool\",\"arguments\":\"not valid json\"}}]},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":5,\"total_tokens\":10}}\n\n" +
		"data: [DONE]\n"

	mockHTTPClient := &http.Client{Transport: &mockTransport{respBody: respBody}}

	config := openrouter.DefaultConfig("test-key")
	config.BaseURL = "https://openrouter.ai/api/v1"
	config.HTTPClient = mockHTTPClient

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "openai/gpt-4o-mini",
		httpClient: openrouter.NewClientWithConfig(*config),
	}

	req := &api.ChatRequest{
		Messages: []api.Message{
			{Role: "user", Content: "Test"},
		},
	}

	var toolCallResp *api.ChatResponse
	err := llm.Chat(context.Background(), req, func(resp api.ChatResponse) error {
		if len(resp.Message.ToolCalls) > 0 {
			toolCallResp = &resp
		}
		return nil
	})

	require.NoError(t, err)
	require.NotNil(t, toolCallResp, "should receive a response with tool calls even if args are malformed")
	require.Len(t, toolCallResp.Message.ToolCalls, 1, "tool call should be preserved")

	assert.Equal(t, "call_bad", toolCallResp.Message.ToolCalls[0].ID, "ID should be preserved")
	assert.Equal(t, "bad_tool", toolCallResp.Message.ToolCalls[0].Function.Name, "Name should be preserved even if args are bad")
}

type headerCapturingTransport struct {
	respBody    string
	capturedReq *http.Request
}

func (m *headerCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.capturedReq = req.Clone(context.Background())
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(m.respBody)),
		Header:     http.Header{},
	}, nil
}

func TestOpenRouterLLM_NoTracePropagation(t *testing.T) {
	respBody := `data: {"id":"gen-123","model":"openai/gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":null}

data: {"id":"gen-123","model":"openai/gpt-4o-mini","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
`

	transport := &headerCapturingTransport{respBody: respBody}

	config := openrouter.DefaultConfig("test-key")
	config.BaseURL = "https://openrouter.ai/api/v1"
	config.HTTPClient = &http.Client{Transport: transport}

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "openai/gpt-4o-mini",
		httpClient: openrouter.NewClientWithConfig(*config),
	}

	req := &api.ChatRequest{
		Messages: []api.Message{
			{Role: "user", Content: "Hi"},
		},
	}

	err := llm.Chat(context.Background(), req, func(resp api.ChatResponse) error {
		return nil
	})

	require.NoError(t, err)
	require.NotNil(t, transport.capturedReq, "request should have been made")

	traceparent := transport.capturedReq.Header.Get("traceparent")
	baggage := transport.capturedReq.Header.Get("baggage")

	assert.Empty(t, traceparent, "traceparent header should NOT be sent to OpenRouter")
	assert.Empty(t, baggage, "baggage header should NOT be sent to OpenRouter")
}

func TestOpenRouterLLM_ConvertMessage_EmptyAssistantMessageBecomesThinking(t *testing.T) {
	llm := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	emptyAssistantMsg := api.Message{
		Role:    conversation.RoleAssistant,
		Content: "",
	}

	converted := llm.convertMessage(emptyAssistantMsg)

	assert.Equal(t, "Thinking...", converted.Content.Text, "empty assistant message should be converted to 'Thinking...'")
	assert.Equal(t, string(conversation.RoleAssistant), converted.Role)
}

func TestOpenRouterLLM_ConvertMessage_NonEmptyAssistantMessageUnchanged(t *testing.T) {
	llm := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	msg := api.Message{
		Role:    conversation.RoleAssistant,
		Content: "Hello, world!",
	}

	converted := llm.convertMessage(msg)

	assert.Equal(t, "Hello, world!", converted.Content.Text)
}

func TestOpenRouterLLM_ConvertMessage_EmptyUserMessageUnchanged(t *testing.T) {
	llm := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "openai/gpt-4o-mini",
	}

	msg := api.Message{
		Role:    conversation.RoleUser,
		Content: "",
	}

	converted := llm.convertMessage(msg)

	assert.Equal(t, "", converted.Content.Text, "empty user message should remain empty")
}
