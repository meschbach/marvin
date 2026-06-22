package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLLM struct {
	fn func(ctx context.Context, req *ChatRequest, onResponse func(ctx context.Context, resp *ChatResponse) error) error
}

func (m *mockLLM) Chat(ctx context.Context, req *ChatRequest, onResponse func(ctx context.Context, resp *ChatResponse) error) error {
	return m.fn(ctx, req, onResponse)
}

func newSuccessLLM() *mockLLM {
	return &mockLLM{fn: func(ctx context.Context, req *ChatRequest, onResponse func(ctx context.Context, resp *ChatResponse) error) error {
		return onResponse(ctx, &ChatResponse{Content: "ok", Done: true})
	}}
}

func newFailLLM(err error) *mockLLM {
	return &mockLLM{fn: func(ctx context.Context, req *ChatRequest, onResponse func(ctx context.Context, resp *ChatResponse) error) error {
		return err
	}}
}

func newEntry(label string, llmInst LLM) ModelEntry {
	rb := NewRampingBreaker(label)
	return ModelEntry{Label: label, Provider: "test", LLM: llmInst, Breaker: rb}
}

func newEntryWithTimeout(label string, llmInst LLM, timeout time.Duration) ModelEntry {
	rb := NewRampingBreakerWithSettings(label, defaultBreakerMaxFailures, timeout)
	return ModelEntry{Label: label, Provider: "test", LLM: llmInst, Breaker: rb}
}

func TestOrderedChain_FirstModelSucceeds(t *testing.T) {
	t.Parallel()
	entry1 := newEntry("model-1", newSuccessLLM())
	entry2 := newEntry("model-2", newSuccessLLM())

	chain, err := NewOrderedChain(t.Context(), []ModelEntry{entry1, entry2}, nil)
	require.NoError(t, err)

	var called int
	err = chain.Chat(t.Context(), &ChatRequest{}, func(ctx context.Context, resp *ChatResponse) error {
		called++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, called)
}

func TestOrderedChain_FirstFailsSecondSucceeds(t *testing.T) {
	t.Parallel()
	var failCount int32
	entry1 := newEntry("model-1", &mockLLM{fn: func(ctx context.Context, req *ChatRequest, onResponse func(ctx context.Context, resp *ChatResponse) error) error {
		atomic.AddInt32(&failCount, 1)
		return &RetryableError{Err: errors.New("500")}
	}})
	entry2 := newEntry("model-2", newSuccessLLM())

	chain, err := NewOrderedChain(t.Context(), []ModelEntry{entry1, entry2}, nil)
	require.NoError(t, err)

	var responses int
	err = chain.Chat(t.Context(), &ChatRequest{}, func(ctx context.Context, resp *ChatResponse) error {
		responses++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&failCount))
	assert.Equal(t, 1, responses)
}

func TestOrderedChain_AllModelsFail(t *testing.T) {
	t.Parallel()
	entry1 := newEntry("model-1", newFailLLM(&RetryableError{Err: errors.New("500")}))
	entry2 := newEntry("model-2", newFailLLM(&RetryableError{Err: errors.New("502")}))

	chain, err := NewOrderedChain(t.Context(), []ModelEntry{entry1, entry2}, nil)
	require.NoError(t, err)

	err = chain.Chat(t.Context(), &ChatRequest{}, func(ctx context.Context, resp *ChatResponse) error {
		return nil
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAllModelsExhausted)
}

func TestOrderedChain_BreakerOpenSkipsToSecond(t *testing.T) {
	t.Parallel()
	entry1 := newEntry("model-1", newFailLLM(&RetryableError{Err: errors.New("500")}))

	for i := 0; i < 5; i++ {
		_, _ = entry1.Breaker.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
			return nil, &RetryableError{Err: errors.New("500")}
		})
	}
	assert.Equal(t, gobreaker.StateOpen, entry1.Breaker.State())

	entry2 := newEntry("model-2", newSuccessLLM())
	chain, err := NewOrderedChain(t.Context(), []ModelEntry{entry1, entry2}, nil)
	require.NoError(t, err)

	var called int
	err = chain.Chat(t.Context(), &ChatRequest{}, func(ctx context.Context, resp *ChatResponse) error {
		called++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, called)
}

func TestOrderedChain_BreakerHalfOpenSucceeds(t *testing.T) {
	t.Parallel()
	entry := newEntryWithTimeout("fast-recover", newSuccessLLM(), 1*time.Millisecond)

	for i := 0; i < 5; i++ {
		_, _ = entry.Breaker.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
			return nil, &RetryableError{Err: errors.New("500")}
		})
	}
	assert.Equal(t, gobreaker.StateOpen, entry.Breaker.State())
	time.Sleep(5 * time.Millisecond)

	chain, err := NewOrderedChain(t.Context(), []ModelEntry{entry}, nil)
	require.NoError(t, err)

	err = chain.Chat(t.Context(), &ChatRequest{}, func(ctx context.Context, resp *ChatResponse) error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, gobreaker.StateClosed, entry.Breaker.State())
}

func TestOrderedChain_BreakerHalfOpenFails(t *testing.T) {
	t.Parallel()
	entry := newEntryWithTimeout("half-open-fail", newFailLLM(&RetryableError{Err: errors.New("500")}), 1*time.Millisecond)

	for i := 0; i < 5; i++ {
		_, _ = entry.Breaker.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
			return nil, &RetryableError{Err: errors.New("500")}
		})
	}
	time.Sleep(5 * time.Millisecond)

	chain, err := NewOrderedChain(t.Context(), []ModelEntry{entry}, nil)
	require.NoError(t, err)

	err = chain.Chat(t.Context(), &ChatRequest{}, func(ctx context.Context, resp *ChatResponse) error {
		return nil
	})
	require.Error(t, err)
}

func TestOrderedChain_RateLimitDoesNotTripBreaker(t *testing.T) {
	t.Parallel()
	entry := newEntry("model-1", newFailLLM(&RateLimitTimingError{RetryAfter: 5 * time.Second, Err: errors.New("429")}))

	for i := 0; i < 10; i++ {
		_, _ = entry.Breaker.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
			return nil, &RateLimitTimingError{RetryAfter: 5 * time.Second, Err: errors.New("429")}
		})
	}

	assert.Equal(t, gobreaker.StateClosed, entry.Breaker.State())
}

func TestOrderedChain_ContextCancellationPropagates(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	entry := newEntry("model-1", newSuccessLLM())
	chain, err := NewOrderedChain(t.Context(), []ModelEntry{entry}, nil)
	require.NoError(t, err)

	err = chain.Chat(ctx, &ChatRequest{}, func(ctx context.Context, resp *ChatResponse) error {
		return nil
	})
	require.Error(t, err)
}

func TestOrderedChain_ConcurrentCallsConsistentState(t *testing.T) {
	t.Parallel()
	entry := newEntry("model-1", newSuccessLLM())
	chain, err := NewOrderedChain(t.Context(), []ModelEntry{entry}, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	var errCount int64
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := chain.Chat(t.Context(), &ChatRequest{}, func(ctx context.Context, resp *ChatResponse) error {
				return nil
			})
			if err != nil {
				atomic.AddInt64(&errCount, 1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(0), errCount)
}

func TestOrderedChain_SingleEntryBreakerTrip(t *testing.T) {
	t.Parallel()
	entry := newEntry("model-1", newFailLLM(&RetryableError{Err: errors.New("500")}))

	for i := 0; i < 5; i++ {
		_, _ = entry.Breaker.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
			return nil, &RetryableError{Err: errors.New("500")}
		})
	}
	assert.Equal(t, gobreaker.StateOpen, entry.Breaker.State())

	chain, err := NewOrderedChain(t.Context(), []ModelEntry{entry}, nil)
	require.NoError(t, err)

	err = chain.Chat(t.Context(), &ChatRequest{}, func(ctx context.Context, resp *ChatResponse) error {
		return nil
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAllModelsExhausted)
}

func TestOrderedChain_EmptyChainReturnsError(t *testing.T) {
	t.Parallel()
	_, err := NewOrderedChain(t.Context(), nil, nil)
	require.Error(t, err)
}

func TestOrderedChain_AccessCheckSkipsDeniedModel(t *testing.T) {
	t.Parallel()
	entry1 := newEntry("denied", newSuccessLLM())
	entry2 := newEntry("allowed", newSuccessLLM())

	var calledLabel string
	chain, err := NewOrderedChain(t.Context(), []ModelEntry{entry1, entry2}, func(label string) bool {
		if label == "denied" {
			return false
		}
		calledLabel = label
		return true
	})
	require.NoError(t, err)

	err = chain.Chat(t.Context(), &ChatRequest{}, func(ctx context.Context, resp *ChatResponse) error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "allowed", calledLabel)
}

func TestOrderedChain_AllModelsDenied(t *testing.T) {
	t.Parallel()
	entry1 := newEntry("model-1", newSuccessLLM())

	chain, err := NewOrderedChain(t.Context(), []ModelEntry{entry1}, func(label string) bool {
		return false
	})
	require.NoError(t, err)

	err = chain.Chat(t.Context(), &ChatRequest{}, func(ctx context.Context, resp *ChatResponse) error {
		return nil
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAllModelsExhausted)
}

func TestRampingBreaker_TokensStartAtOneOnRecovery(t *testing.T) {
	t.Parallel()
	rb := NewRampingBreakerWithSettings("ramp-test", defaultBreakerMaxFailures, 1*time.Millisecond)

	for i := 0; i < 5; i++ {
		_, _ = rb.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
			return nil, &RetryableError{Err: errors.New("500")}
		})
	}
	assert.Equal(t, gobreaker.StateOpen, rb.State())
	time.Sleep(5 * time.Millisecond)

	_, _ = rb.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
		return &ChatResponse{Content: "ok"}, nil
	})

	assert.True(t, rb.IsRecovering())
	assert.Less(t, rb.Tokens(), defaultMaxTokens)
}

func TestRampingBreaker_TokensDoubleOnSuccess(t *testing.T) {
	t.Parallel()
	rb := NewRampingBreakerWithSettings("ramp-double", defaultBreakerMaxFailures, 1*time.Millisecond)

	for i := 0; i < 5; i++ {
		_, _ = rb.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
			return nil, &RetryableError{Err: errors.New("500")}
		})
	}
	time.Sleep(5 * time.Millisecond)

	_, err := rb.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
		return &ChatResponse{Content: "ok"}, nil
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, rb.Tokens(), int64(2))
}

func TestRampingBreaker_FailureDuringRampResetsTokens(t *testing.T) {
	t.Parallel()
	rb := NewRampingBreakerWithSettings("ramp-reset", defaultBreakerMaxFailures, 1*time.Millisecond)

	for i := 0; i < 5; i++ {
		_, _ = rb.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
			return nil, &RetryableError{Err: errors.New("500")}
		})
	}
	time.Sleep(5 * time.Millisecond)

	_, _ = rb.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
		return &ChatResponse{Content: "ok"}, nil
	})
	assert.True(t, rb.IsRecovering())

	_, _ = rb.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
		return nil, &RetryableError{Err: errors.New("500")}
	})

	assert.Equal(t, int64(1), rb.Tokens())
}

func TestRampingBreaker_RateLimitGateBlocksRequests(t *testing.T) {
	t.Parallel()
	rb := NewRampingBreaker("rate-limit-gate")

	_, err := rb.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
		return nil, &RateLimitTimingError{RetryAfter: 5 * time.Second, Err: errors.New("429")}
	})
	require.ErrorIs(t, err, ErrRateLimited)

	_, err = rb.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
		return &ChatResponse{Content: "should not reach"}, nil
	})
	require.ErrorIs(t, err, ErrRateLimited)
}

func TestRampingBreaker_RateLimitGateExpiresNaturally(t *testing.T) {
	t.Parallel()
	rb := NewRampingBreaker("rate-limit-expire")

	_, err := rb.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
		return nil, &RateLimitTimingError{RetryAfter: 1 * time.Millisecond, Err: errors.New("429")}
	})
	require.ErrorIs(t, err, ErrRateLimited)

	time.Sleep(5 * time.Millisecond)

	_, err = rb.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
		return &ChatResponse{Content: "ok"}, nil
	})
	require.NoError(t, err)
}

func TestRampingBreaker_ErrNoTokenMultiEntryFallsThrough(t *testing.T) {
	t.Parallel()
	rb1 := NewRampingBreakerWithSettings("no-token-1", defaultBreakerMaxFailures, 1*time.Millisecond)

	for i := 0; i < 5; i++ {
		_, _ = rb1.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
			return nil, &RetryableError{Err: errors.New("500")}
		})
	}
	time.Sleep(5 * time.Millisecond)

	_, _ = rb1.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
		return &ChatResponse{Content: "ok"}, nil
	})
	assert.True(t, rb1.IsRecovering())

	entry1 := ModelEntry{Label: "recovering", LLM: newSuccessLLM(), Breaker: rb1}
	entry2 := ModelEntry{Label: "fallback", LLM: newSuccessLLM(), Breaker: NewRampingBreaker("fallback")}

	chain, err := NewOrderedChain(t.Context(), []ModelEntry{entry1, entry2}, nil)
	require.NoError(t, err)

	err = chain.Chat(t.Context(), &ChatRequest{}, func(ctx context.Context, resp *ChatResponse) error {
		return nil
	})
	require.NoError(t, err)
}

func TestOrderedChain_PermanentErrorDoesNotTripBreaker(t *testing.T) {
	t.Parallel()
	entry := newEntry("model-1", newFailLLM(&PermanentError{Err: errors.New("401")}))

	for i := 0; i < 10; i++ {
		_, _ = entry.Breaker.Execute(t.Context(), func(ctx context.Context) (*ChatResponse, error) {
			return nil, &PermanentError{Err: fmt.Errorf("401")}
		})
	}

	assert.Equal(t, gobreaker.StateClosed, entry.Breaker.State())
}
