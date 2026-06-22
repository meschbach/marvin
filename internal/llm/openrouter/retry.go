package openrouter

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/meschbach/marvin/internal/config"
	"github.com/revrost/go-openrouter"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const rateLimitStatusCode = 429
const rateLimitResetKey = "X-RateLimit-Reset"

// parseRateLimitResetValue extracts a millisecond duration from a rate-limit reset
// header value, which may be a float64 (from JSON decoding) or a string.
func parseRateLimitResetValue(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, val > 0
	case string:
		parsed, err := strconv.ParseFloat(val, 64)
		return parsed, err == nil && parsed > 0
	default:
		return 0, false
	}
}

func extractRateLimitReset(err error) (time.Duration, bool) {
	var apiErr *openrouter.APIError
	if !errors.As(err, &apiErr) || apiErr.Metadata == nil || *apiErr.Metadata == nil {
		return 0, false
	}

	resetValue, exists := (*apiErr.Metadata)[rateLimitResetKey]
	if !exists {
		return 0, false
	}

	ms, ok := parseRateLimitResetValue(resetValue)
	if !ok {
		return 0, false
	}

	return time.Duration(ms) * time.Millisecond, true
}

func isRetryableError(err error) bool {
	var apiErr *openrouter.APIError
	ok := errors.As(err, &apiErr)
	if !ok {
		return false
	}
	return apiErr.HTTPStatusCode == rateLimitStatusCode || (apiErr.HTTPStatusCode >= 500 && apiErr.HTTPStatusCode < 600)
}

func deriveErrorStatus(err error) (errorType string, httpStatus int) {
	var apiErr *openrouter.APIError
	if errors.As(err, &apiErr) {
		return "api_error", apiErr.HTTPStatusCode
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout", 0
	}
	if errors.Is(err, context.Canceled) {
		return "canceled", 0
	}
	return "error", 0
}

type metricsRecorder struct {
	startedCounter           metric.Int64Counter
	requestCounter           metric.Int64Counter
	retryCounter             metric.Int64Counter
	rateLimitExceededCounter metric.Int64Counter
	latencyHistogram         metric.Float64Histogram
	waitHistogram            metric.Float64Histogram
	errorCounter             metric.Int64Counter
}

func newMetricsRecorder(meter metric.Meter) (*metricsRecorder, error) {
	startedCounter, err := meter.Int64Counter("llm.requests.started",
		metric.WithDescription("Number of LLM requests initiated"),
		metric.WithUnit("1"))
	if err != nil {
		return nil, err
	}

	requestCounter, err := meter.Int64Counter("llm.requests.total",
		metric.WithDescription("Total number of LLM requests"),
		metric.WithUnit("1"))
	if err != nil {
		return nil, err
	}

	retryCounter, err := meter.Int64Counter("llm.rate_limit_retries",
		metric.WithDescription("Number of retries due to rate limiting"),
		metric.WithUnit("1"))
	if err != nil {
		return nil, err
	}

	rateLimitExceededCounter, err := meter.Int64Counter("llm.rate_limit_wait_exceeded",
		metric.WithDescription("Number of times rate limit reset time exceeded max wait"),
		metric.WithUnit("1"))
	if err != nil {
		return nil, err
	}

	latencyHistogram, err := meter.Float64Histogram("llm.requests.latency",
		metric.WithDescription("Latency of LLM requests in seconds"),
		metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}

	waitHistogram, err := meter.Float64Histogram("llm.rate_limit_wait_seconds",
		metric.WithDescription("Wait time between retries in seconds"),
		metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}

	errorCounter, err := meter.Int64Counter("llm.errors.total",
		metric.WithDescription("Total number of LLM errors"),
		metric.WithUnit("1"))
	if err != nil {
		return nil, err
	}

	return &metricsRecorder{
		startedCounter:           startedCounter,
		requestCounter:           requestCounter,
		retryCounter:             retryCounter,
		rateLimitExceededCounter: rateLimitExceededCounter,
		latencyHistogram:         latencyHistogram,
		waitHistogram:            waitHistogram,
		errorCounter:             errorCounter,
	}, nil
}

func (m *metricsRecorder) recordStarted(ctx context.Context, provider, model string) {
	if m == nil {
		return
	}
	m.startedCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model),
	))
}

func (m *metricsRecorder) recordRequest(ctx context.Context, provider, model, outcome string, retryed bool) {
	if m == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("provider", provider),
		attribute.String("model", model),
		attribute.String("outcome", outcome),
	}
	if retryed {
		attrs = append(attrs, attribute.String("retryed", "true"))
	}
	m.requestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
}

func (m *metricsRecorder) recordRetry(ctx context.Context, provider, model string) {
	if m == nil {
		return
	}
	m.retryCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model),
	))
}

func (m *metricsRecorder) recordLatency(ctx context.Context, provider, model, outcome string, durationSeconds float64) {
	if m == nil {
		return
	}
	m.latencyHistogram.Record(ctx, durationSeconds, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model),
		attribute.String("outcome", outcome),
	))
}

func (m *metricsRecorder) recordWaitTime(ctx context.Context, provider, model string, waitSeconds float64, waitSource string) {
	if m == nil {
		return
	}
	m.waitHistogram.Record(ctx, waitSeconds, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model),
		attribute.String("wait_source", waitSource),
	))
}

func (m *metricsRecorder) recordRateLimitExceeded(ctx context.Context, provider, model string) {
	if m == nil {
		return
	}
	m.rateLimitExceededCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model),
	))
}

func (m *metricsRecorder) recordError(ctx context.Context, provider, model, errorType string, httpStatus int) {
	if m == nil {
		return
	}
	m.errorCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model),
		attribute.String("error.type", errorType),
		attribute.Int("http.status", httpStatus),
	))
}

func (m *metricsRecorder) recordTerminalError(ctx context.Context, provider, model, errorType string, httpStatus int, durationSeconds float64, wasRetried bool) {
	if m == nil {
		return
	}
	m.recordError(ctx, provider, model, errorType, httpStatus)
	m.recordRequest(ctx, provider, model, "error", wasRetried)
	m.recordLatency(ctx, provider, model, "error", durationSeconds)
}

// retryConfig holds the resolved retry parameters for stream creation.
type retryConfig struct {
	maxRetries       int
	maxRateLimitWait time.Duration
	backoffMgr       backoff.BackOff
}

// loadRetryConfig resolves retry parameters from the LLM's configuration or applies defaults.
func (o *LLM) loadRetryConfig() (retryConfig, error) {
	maxRetries := 3
	if o.retryConfig != nil {
		var errVal error
		maxRetries, errVal = o.retryConfig.MaxAttemptsValue()
		if errVal != nil {
			return retryConfig{}, fmt.Errorf("max_attempts: %w", errVal)
		}
	}

	maxRateLimitWait := config.DefaultMaxRateLimitWait
	if o.retryConfig != nil {
		var errVal error
		maxRateLimitWait, errVal = o.retryConfig.MaxRateLimitWaitValue()
		if errVal != nil {
			return retryConfig{}, fmt.Errorf("max_rate_limit_wait: %w", errVal)
		}
	}

	backoffMgr, errVal := o.getBackoff()
	if errVal != nil {
		return retryConfig{}, fmt.Errorf("backoff config: %w", errVal)
	}

	return retryConfig{
		maxRetries:       maxRetries,
		maxRateLimitWait: maxRateLimitWait,
		backoffMgr:       backoffMgr,
	}, nil
}

type streamCreator func(ctx context.Context) (*openrouter.ChatCompletionStream, error)

// waitWithBackoff pauses for the given duration or until context cancellation.
// Returns nil after the duration elapses, or ctx.Err() if canceled.
func waitWithBackoff(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}

func (o *LLM) executeWithRetry(ctx context.Context, createStream streamCreator) (*openrouter.ChatCompletionStream, error) {
	startTime := time.Now()
	provider := "openrouter"
	model := o.model

	cfg, err := o.loadRetryConfig()
	if err != nil {
		return nil, err
	}

	o.metrics.recordStarted(ctx, provider, model)

	var attempt int
	var stream *openrouter.ChatCompletionStream

	for attempt = 0; attempt < cfg.maxRetries; attempt++ {
		stream, err = createStream(ctx)
		if err == nil {
			break
		}

		if !isRetryableError(err) {
			return nil, o.terminalError(ctx, provider, model, err, startTime, attempt)
		}

		resetWait, hasResetWait := extractRateLimitReset(err)
		if hasResetWait {
			if err := o.handleRateLimitWait(ctx, provider, model, resetWait, cfg.maxRateLimitWait, err, startTime, attempt); err != nil {
				return nil, err
			}
			continue
		}

		if !o.doExponentialBackoff(ctx, provider, model, cfg.backoffMgr) {
			break
		}
	}

	return o.finalResult(ctx, provider, model, stream, err, startTime, attempt)
}

// terminalError records a terminal error metric and returns the original error.
func (o *LLM) terminalError(ctx context.Context, provider, model string, err error, startTime time.Time, attempt int) error {
	duration := time.Since(startTime).Seconds()
	errorType, httpStatus := deriveErrorStatus(err)
	o.metrics.recordTerminalError(ctx, provider, model, errorType, httpStatus, duration, attempt > 0)
	return err
}

// handleRateLimitWait handles a rate-limit-reset response. It returns nil if the
// caller should retry, or an error if the wait was too long or context was canceled.
func (o *LLM) handleRateLimitWait(ctx context.Context, provider, model string, resetWait, maxRateLimitWait time.Duration, originalErr error, startTime time.Time, attempt int) error {
	if resetWait <= maxRateLimitWait {
		if err := waitWithBackoff(ctx, resetWait); err != nil {
			return err
		}
		o.metrics.recordWaitTime(ctx, provider, model, resetWait.Seconds(), "server_reset")
		o.metrics.recordRetry(ctx, provider, model)
		return nil
	}

	o.metrics.recordRateLimitExceeded(ctx, provider, model)
	duration := time.Since(startTime).Seconds()
	errorType, httpStatus := deriveErrorStatus(originalErr)
	o.metrics.recordTerminalError(ctx, provider, model, errorType, httpStatus, duration, attempt > 0)
	return fmt.Errorf("rate limit reset time %v exceeds configured max_wait %v; consider increasing max_rate_limit_wait in your retry block: %w", resetWait, maxRateLimitWait, originalErr)
}

// doExponentialBackoff waits using the backoff manager. Returns true if the caller
// should retry, or false if the backoff is exhausted or context was canceled.
func (o *LLM) doExponentialBackoff(ctx context.Context, provider, model string, backoffMgr backoff.BackOff) bool {
	waitTime := backoffMgr.NextBackOff()
	if waitTime == backoff.Stop {
		return false
	}
	if err := waitWithBackoff(ctx, waitTime); err != nil {
		return false
	}
	o.metrics.recordWaitTime(ctx, provider, model, waitTime.Seconds(), "exponential_backoff")
	o.metrics.recordRetry(ctx, provider, model)
	return true
}

// finalResult records final outcome metrics and returns the appropriate result.
func (o *LLM) finalResult(ctx context.Context, provider, model string, stream *openrouter.ChatCompletionStream, err error, startTime time.Time, attempt int) (*openrouter.ChatCompletionStream, error) {
	duration := time.Since(startTime).Seconds()
	if err != nil {
		errorType, httpStatus := deriveErrorStatus(err)
		o.metrics.recordTerminalError(ctx, provider, model, errorType, httpStatus, duration, attempt > 0)
		return nil, fmt.Errorf("retry exhausted after %d attempts: %w", attempt+1, err)
	}
	o.metrics.recordRequest(ctx, provider, model, "success", attempt > 0)
	o.metrics.recordLatency(ctx, provider, model, "success", duration)
	return stream, nil
}
