package llm

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for chain-level control flow.
var (
	ErrRateLimited        = errors.New("model is rate limited")
	ErrNoToken            = errors.New("recovery ramp at capacity")
	ErrAllModelsExhausted = errors.New("all models in chain failed")
)

// IsRetryable reports whether err is a retryable failure.
func IsRetryable(err error) bool {
	var r interface{ IsRetryable() bool }
	if errors.As(err, &r) {
		return r.IsRetryable()
	}
	return false
}

// IsPermanent reports whether err is a permanent (non-retryable) failure.
func IsPermanent(err error) bool {
	var p interface{ IsPermanent() bool }
	if errors.As(err, &p) {
		return p.IsPermanent()
	}
	return false
}

// RetryableError represents a transient error that can be retried.
// Examples: 429 (no timing), 5xx, network timeouts.
type RetryableError struct {
	Err     error
	Message string
}

func (e *RetryableError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("retryable: %v", e.Err)
}

func (e *RetryableError) Unwrap() error { return e.Err }

func (e *RetryableError) IsRetryable() bool { return true }

func (e *RetryableError) IsPermanent() bool { return false }

// PermanentError represents a non-recoverable error.
// Examples: 401, 404, 400 (bad request).
type PermanentError struct {
	Err     error
	Message string
}

func (e *PermanentError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("permanent: %v", e.Err)
}

func (e *PermanentError) Unwrap() error { return e.Err }

func (e *PermanentError) IsRetryable() bool { return false }

func (e *PermanentError) IsPermanent() bool { return true }

// RateLimitTimingError wraps a 429 response that includes Retry-After or
// X-RateLimit-Reset timing information. The chain uses this to set the
// rate-limit gate without tripping the circuit breaker.
type RateLimitTimingError struct {
	RetryAfter time.Duration
	Err        error
	Message    string
}

func (e *RateLimitTimingError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("rate limited, retry after %v: %v", e.RetryAfter, e.Err)
}

func (e *RateLimitTimingError) Unwrap() error { return e.Err }

func (e *RateLimitTimingError) IsRetryable() bool { return true }

func (e *RateLimitTimingError) IsPermanent() bool { return false }
