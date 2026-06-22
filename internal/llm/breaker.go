package llm

import (
	"time"

	"github.com/sony/gobreaker/v2"
)

const (
	defaultBreakerMaxFailures = 5
	defaultBreakerTimeout     = 60 * time.Second
)

// BreakerSettings configures a per-model circuit breaker.
type BreakerSettings struct {
	Name          string
	MaxFailures   uint32
	Timeout       time.Duration
	OnStateChange func(name string, from, to gobreaker.State)
}

// NewCircuitBreaker creates a circuit breaker for a single model entry.
// 429s do NOT trip the breaker — rate limits are load signals, not health signals.
// The breaker only trips on health failures (5xx, timeout, connection errors).
func NewCircuitBreaker(s BreakerSettings) *gobreaker.CircuitBreaker[*ChatResponse] {
	if s.MaxFailures == 0 {
		s.MaxFailures = defaultBreakerMaxFailures
	}
	if s.Timeout == 0 {
		s.Timeout = defaultBreakerTimeout
	}

	settings := gobreaker.Settings{
		Name:        s.Name,
		MaxRequests: 1,
		Interval:    0,
		Timeout:     s.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= uint32(s.MaxFailures)
		},
		OnStateChange: s.OnStateChange,
	}

	return gobreaker.NewCircuitBreaker[*ChatResponse](settings)
}
