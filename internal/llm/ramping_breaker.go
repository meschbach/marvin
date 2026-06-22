package llm

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/sony/gobreaker/v2"
)

const (
	stateSteady      int32 = 0
	stateRecovering  int32 = 1
	defaultMaxTokens int64 = 50
)

// CircuitEventCallback is called when a circuit breaker changes state.
type CircuitEventCallback func(model string, from, to gobreaker.State)

// RampingBreaker wraps a circuit breaker with token-bucket slow-start
// and a rate-limit gate. After recovery from an open breaker, traffic
// is gradually re-admitted by doubling tokens per round until reaching
// capacity. The rate-limit gate blocks requests during known congestion
// windows (429 with timing info).
type RampingBreaker struct {
	cb                   *gobreaker.CircuitBreaker[*ChatResponse]
	maxTokens            int64
	tokens               int64
	state                int32
	rateLimitUntil       atomic.Int64
	tokenAvail           chan struct{}
	name                 string
	circuitEventCallback CircuitEventCallback
}

// NewRampingBreaker creates a RampingBreaker with an internally created circuit breaker.
// The breaker's OnStateChange callback is wired to seed the recovery ramp.
func NewRampingBreaker(name string) *RampingBreaker {
	return NewRampingBreakerWithSettings(name, defaultBreakerMaxFailures, defaultBreakerTimeout)
}

// NewRampingBreakerWithSettings creates a RampingBreaker with custom breaker settings.
func NewRampingBreakerWithSettings(name string, maxFailures uint32, timeout time.Duration) *RampingBreaker {
	rb := &RampingBreaker{
		maxTokens:  defaultMaxTokens,
		tokens:     defaultMaxTokens,
		state:      stateSteady,
		tokenAvail: make(chan struct{}, 1),
		name:       name,
	}

	settings := gobreaker.Settings{
		Name:        name,
		MaxRequests: 1,
		Timeout:     timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= maxFailures
		},
		OnStateChange: func(breakerName string, from, to gobreaker.State) {
			if from == gobreaker.StateHalfOpen && to == gobreaker.StateClosed {
				atomic.StoreInt64(&rb.tokens, 1)
				atomic.StoreInt32(&rb.state, stateRecovering)
			}
			if rb.circuitEventCallback != nil {
				rb.circuitEventCallback(breakerName, from, to)
			}
		},
	}
	rb.cb = gobreaker.NewCircuitBreaker[*ChatResponse](settings)
	return rb
}

// SetCircuitEventCallback sets a callback for circuit breaker state changes.
func (r *RampingBreaker) SetCircuitEventCallback(cb CircuitEventCallback) {
	r.circuitEventCallback = cb
}

// Execute runs the request through the breaker with rate-limit gating
// and recovery ramp admission control.
func (r *RampingBreaker) Execute(ctx context.Context, req func(ctx context.Context) (*ChatResponse, error)) (*ChatResponse, error) {
	if until := r.rateLimitUntil.Load(); until > 0 && until > time.Now().UnixNano() {
		return nil, ErrRateLimited
	}

	if atomic.LoadInt32(&r.state) == stateRecovering {
		if !r.acquireToken(ctx) {
			return nil, ErrNoToken
		}
	}

	var capturedRateLimit bool
	result, err := r.cb.Execute(func() (*ChatResponse, error) {
		resp, callErr := req(ctx)

		var rateLimitErr *RateLimitTimingError
		if errors.As(callErr, &rateLimitErr) {
			r.setRateLimitWindow(rateLimitErr)
			capturedRateLimit = true
			return nil, nil
		}

		var permErr *PermanentError
		if errors.As(callErr, &permErr) {
			return nil, nil
		}

		return resp, callErr
	})

	if capturedRateLimit {
		return nil, ErrRateLimited
	}

	r.admit(err)
	return result, err
}

func (r *RampingBreaker) acquireToken(ctx context.Context) bool {
	if r.tryAcquireToken() {
		return true
	}
	select {
	case <-r.tokenAvail:
		return r.tryAcquireToken()
	case <-time.After(50 * time.Millisecond):
		return false
	case <-ctx.Done():
		return false
	}
}

func (r *RampingBreaker) tryAcquireToken() bool {
	for {
		current := atomic.LoadInt64(&r.tokens)
		if current <= 0 {
			return false
		}
		if atomic.CompareAndSwapInt64(&r.tokens, current, current-1) {
			return true
		}
	}
}

func (r *RampingBreaker) admit(err error) {
	if atomic.LoadInt32(&r.state) != stateRecovering {
		r.releaseToken()
		return
	}

	if err != nil {
		atomic.StoreInt64(&r.tokens, 1)
		return
	}

	for {
		current := atomic.LoadInt64(&r.tokens)
		next := current * 2
		if next > r.maxTokens {
			next = r.maxTokens
		}
		if atomic.CompareAndSwapInt64(&r.tokens, current, next) {
			if next >= r.maxTokens {
				atomic.StoreInt32(&r.state, stateSteady)
				atomic.StoreInt64(&r.tokens, r.maxTokens)
			}
			break
		}
	}
	r.releaseToken()
}

func (r *RampingBreaker) releaseToken() {
	select {
	case r.tokenAvail <- struct{}{}:
	default:
	}
}

func (r *RampingBreaker) setRateLimitWindow(err *RateLimitTimingError) {
	r.rateLimitUntil.Store(time.Now().Add(err.RetryAfter).UnixNano())
}

// State returns the current breaker state for testing.
func (r *RampingBreaker) State() gobreaker.State {
	return r.cb.State()
}

// Tokens returns the current token count for testing.
func (r *RampingBreaker) Tokens() int64 {
	return atomic.LoadInt64(&r.tokens)
}

// IsRecovering reports whether the breaker is in recovery ramp state.
func (r *RampingBreaker) IsRecovering() bool {
	return atomic.LoadInt32(&r.state) == stateRecovering
}
