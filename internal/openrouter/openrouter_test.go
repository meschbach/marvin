package openrouter

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ollama/ollama/api"
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

	mockClient := &http.Client{Transport: &mockTransport{respBody: respBody}}

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "openai/gpt-4o-mini",
		httpClient: mockClient,
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

	mockClient := &http.Client{Transport: &mockTransport{respBody: respBody}}

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "openai/gpt-4o-mini",
		httpClient: mockClient,
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

	mockClient := &http.Client{Transport: &mockTransport{respBody: respBody}}

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "openai/gpt-4o-mini",
		httpClient: mockClient,
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

	mockClient := &http.Client{Transport: &mockTransport{respBody: respBody}}

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "nvidia/nemotron-3-nano-30b-a3b:free",
		httpClient: mockClient,
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
