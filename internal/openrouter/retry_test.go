package openrouter

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/meschbach/marvin/internal/config"
	"github.com/ollama/ollama/api"
	"github.com/revrost/go-openrouter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsRetryableError_429(t *testing.T) {
	t.Parallel()
	err := &openrouter.APIError{
		HTTPStatusCode: 429,
		Message:        "Rate limit exceeded",
	}
	assert.True(t, isRetryableError(err), "429 should be retryable")
}

func TestIsRetryableError_500(t *testing.T) {
	t.Parallel()
	err := &openrouter.APIError{
		HTTPStatusCode: 500,
		Message:        "Internal server error",
	}
	assert.True(t, isRetryableError(err), "500 should be retryable")
}

func TestIsRetryableError_502(t *testing.T) {
	t.Parallel()
	err := &openrouter.APIError{
		HTTPStatusCode: 502,
		Message:        "Bad gateway",
	}
	assert.True(t, isRetryableError(err), "502 should be retryable")
}

func TestIsRetryableError_503(t *testing.T) {
	t.Parallel()
	err := &openrouter.APIError{
		HTTPStatusCode: 503,
		Message:        "Service unavailable",
	}
	assert.True(t, isRetryableError(err), "503 should be retryable")
}

func TestIsRetryableError_504(t *testing.T) {
	t.Parallel()
	err := &openrouter.APIError{
		HTTPStatusCode: 504,
		Message:        "Gateway timeout",
	}
	assert.True(t, isRetryableError(err), "504 should be retryable")
}

func TestIsRetryableError_400(t *testing.T) {
	t.Parallel()
	err := &openrouter.APIError{
		HTTPStatusCode: 400,
		Message:        "Bad request",
	}
	assert.False(t, isRetryableError(err), "400 should not be retryable")
}

func TestIsRetryableError_401(t *testing.T) {
	t.Parallel()
	err := &openrouter.APIError{
		HTTPStatusCode: 401,
		Message:        "Unauthorized",
	}
	assert.False(t, isRetryableError(err), "401 should not be retryable")
}

func TestIsRetryableError_NonAPIError(t *testing.T) {
	t.Parallel()
	err := assert.AnError
	assert.False(t, isRetryableError(err), "non-APIError should not be retryable")
}

func TestParseRetryAfterHeader_Seconds(t *testing.T) {
	t.Parallel()
	resp := &http.Response{
		Header: http.Header{
			"Retry-After": []string{"30"},
		},
	}
	duration := parseRetryAfterHeader(resp)
	assert.Equal(t, 30*time.Second, duration)
}

func TestParseRetryAfterHeader_ZeroValue(t *testing.T) {
	t.Parallel()
	resp := &http.Response{
		Header: http.Header{},
	}
	duration := parseRetryAfterHeader(resp)
	assert.Equal(t, time.Duration(0), duration)
}

func TestParseRetryAfterHeader_InvalidValue(t *testing.T) {
	t.Parallel()
	resp := &http.Response{
		Header: http.Header{
			"Retry-After": []string{"not-a-number"},
		},
	}
	duration := parseRetryAfterHeader(resp)
	assert.Equal(t, time.Duration(0), duration)
}

func TestRetryConfig_DefaultValues(t *testing.T) {
	t.Parallel()
	cfg := &config.RetryBlock{}
	assert.Equal(t, config.DefaultMaxRetries, cfg.MaxRetriesValue())
	assert.Equal(t, config.DefaultInitialInterval, cfg.InitialIntervalValue())
	assert.Equal(t, config.DefaultMaxInterval, cfg.MaxIntervalValue())
}

func TestRetryConfig_CustomValues(t *testing.T) {
	t.Parallel()
	initialInterval := 2 * time.Second
	maxInterval := 60 * time.Second
	maxRetries := 5
	cfg := &config.RetryBlock{
		MaxRetries:      &maxRetries,
		InitialInterval: &initialInterval,
		MaxInterval:     &maxInterval,
	}
	assert.Equal(t, 5, cfg.MaxRetriesValue())
	assert.Equal(t, 2*time.Second, cfg.InitialIntervalValue())
	assert.Equal(t, 60*time.Second, cfg.MaxIntervalValue())
}

type retryableTransport struct {
	t                *testing.T
	attempts         int
	maxAttempts      int
	successOnAttempt int
	responseBody     string
}

func (rt *retryableTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.attempts++
	if rt.attempts < rt.successOnAttempt {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"rate_limit","message":"Rate limit"}}`)),
			Header:     http.Header{},
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(rt.responseBody)),
		Header:     http.Header{},
	}, nil
}

func TestExecuteWithRetry_SucceedsOnSecondAttempt(t *testing.T) {
	t.Parallel()
	respBody := `data: {"id":"gen-123","model":"test","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":null}

data: {"id":"gen-123","model":"test","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
`

	transport := &retryableTransport{
		t:                t,
		successOnAttempt: 2,
		maxAttempts:      3,
		responseBody:     respBody,
	}

	mockHTTPClient := &http.Client{Transport: transport}

	config := openrouter.DefaultConfig("test-key")
	config.BaseURL = "https://openrouter.ai/api/v1"
	config.HTTPClient = mockHTTPClient

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "test-model",
		httpClient: openrouter.NewClientWithConfig(*config),
	}

	stream, err := llm.executeWithRetry(t.Context(), func(ctx context.Context) (*openrouter.ChatCompletionStream, error) {
		return llm.httpClient.CreateChatCompletionStream(ctx, openrouter.ChatCompletionRequest{
			Model: "test",
			Messages: []openrouter.ChatCompletionMessage{
				{Role: "user", Content: openrouter.Content{Text: "test"}},
			},
			Stream: true,
		})
	})

	require.NoError(t, err)
	require.NotNil(t, stream)
	assert.Equal(t, 2, transport.attempts, "should have attempted twice")
	stream.Close()
}

func TestExecuteWithRetry_ExhaustsRetries(t *testing.T) {
	t.Parallel()
	transport := &retryableTransport{
		t:                t,
		successOnAttempt: 10, // never succeeds
		maxAttempts:      3,
		responseBody:     `data: [DONE]`,
	}

	mockHTTPClient := &http.Client{Transport: transport}

	config := openrouter.DefaultConfig("test-key")
	config.BaseURL = "https://openrouter.ai/api/v1"
	config.HTTPClient = mockHTTPClient

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "test-model",
		httpClient: openrouter.NewClientWithConfig(*config),
	}

	stream, err := llm.executeWithRetry(t.Context(), func(ctx context.Context) (*openrouter.ChatCompletionStream, error) {
		return llm.httpClient.CreateChatCompletionStream(ctx, openrouter.ChatCompletionRequest{
			Model: "test",
			Messages: []openrouter.ChatCompletionMessage{
				{Role: "user", Content: openrouter.Content{Text: "test"}},
			},
			Stream: true,
		})
	})

	assert.Error(t, err, "should return error after retries exhausted")
	assert.Nil(t, stream)
	assert.Equal(t, 3, transport.attempts, "should have attempted 3 times")
}

func TestExecuteWithRetry_NonRetryableError(t *testing.T) {
	t.Parallel()
	errResp := `{"error":{"code":"invalid_request_error","message":"Invalid model"}}`
	transport := &mockTransportWithStatus{
		statusCode: http.StatusBadRequest,
		mockTransport: mockTransport{
			respBody: errResp,
		},
	}

	mockHTTPClient := &http.Client{Transport: transport}

	config := openrouter.DefaultConfig("test-key")
	config.BaseURL = "https://openrouter.ai/api/v1"
	config.HTTPClient = mockHTTPClient

	llm := &LLM{
		apiKey:     "test-key",
		baseURL:    "https://openrouter.ai/api/v1",
		model:      "test-model",
		httpClient: openrouter.NewClientWithConfig(*config),
	}

	stream, err := llm.executeWithRetry(t.Context(), func(ctx context.Context) (*openrouter.ChatCompletionStream, error) {
		return llm.httpClient.CreateChatCompletionStream(ctx, openrouter.ChatCompletionRequest{
			Model: "invalid-model",
			Messages: []openrouter.ChatCompletionMessage{
				{Role: "user", Content: openrouter.Content{Text: "test"}},
			},
			Stream: true,
		})
	})

	assert.Error(t, err, "non-retryable error should not be retried")
	assert.Nil(t, stream)
}

type mockTransportWithStatus struct {
	mockTransport
	statusCode int
}

func (m *mockTransportWithStatus) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(strings.NewReader(m.respBody)),
		Header:     http.Header{},
	}, nil
}

type capturedResponseCollector struct {
	responses []api.ChatResponse
	last      *api.ChatResponse
	err       error
	count     int
}

func (rc *capturedResponseCollector) OnChatResponse(ctx context.Context, resp *api.ChatResponse) error {
	rc.count++
	rc.responses = append(rc.responses, *resp)
	rc.last = resp
	return rc.err
}

func TestLLM_Chat_WithRetry_SuccessAfterRateLimit(t *testing.T) {
	t.Parallel()
	respBody := `data: {"id":"gen-123","model":"test","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":null}

data: {"id":"gen-123","model":"test","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
`

	callCount := 0
	var transport http.RoundTripper = &roundTripperFunc{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			callCount++
			if callCount == 1 {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"rate_limit","message":"Rate limit"}}`)),
					Header:     http.Header{"Retry-After": []string{"0"}},
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(respBody)),
				Header:     http.Header{},
			}, nil
		},
	}

	mockHTTPClient := &http.Client{Transport: transport}

	openrouterConfig := openrouter.DefaultConfig("test-key")
	openrouterConfig.BaseURL = "https://openrouter.ai/api/v1"
	openrouterConfig.HTTPClient = mockHTTPClient

	maxRetries := 3
	llm := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "test-model",
		retryConfig: &config.RetryBlock{
			MaxRetries: &maxRetries,
		},
		httpClient: openrouter.NewClientWithConfig(*openrouterConfig),
	}

	req := &api.ChatRequest{
		Messages: []api.Message{
			{Role: "user", Content: "Hi"},
		},
	}

	collector := &capturedResponseCollector{}
	err := llm.Chat(t.Context(), req, collector)

	require.NoError(t, err, "should succeed after retry")
	assert.True(t, collector.last.Done, "last response should be done")
}

type roundTripperFunc struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (f *roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f.roundTrip(req)
}

func TestLLM_Chat_WithRetry_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	respBody := `data: {"id":"gen-123","model":"test","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":null}

data: {"id":"gen-123","model":"test","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
`

	transport := &mockTransport{respBody: respBody}
	mockHTTPClient := &http.Client{Transport: transport}

	openrouterConfig := openrouter.DefaultConfig("test-key")
	openrouterConfig.BaseURL = "https://openrouter.ai/api/v1"
	openrouterConfig.HTTPClient = mockHTTPClient

	maxRetries := 3
	llm := &LLM{
		apiKey:  "test-key",
		baseURL: "https://openrouter.ai/api/v1",
		model:   "test-model",
		retryConfig: &config.RetryBlock{
			MaxRetries: &maxRetries,
		},
		httpClient: openrouter.NewClientWithConfig(*openrouterConfig),
	}

	req := &api.ChatRequest{
		Messages: []api.Message{
			{Role: "user", Content: "Hi"},
		},
	}

	collector := &capturedResponseCollector{}
	err := llm.Chat(t.Context(), req, collector)

	require.NoError(t, err)
	require.NotEmpty(t, collector.responses)
	assert.True(t, collector.last.Done)
}
