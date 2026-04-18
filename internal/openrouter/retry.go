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

func extractRateLimitReset(err error) (time.Duration, bool) {
	var apiErr *openrouter.APIError
	if !errors.As(err, &apiErr) || apiErr.Metadata == nil || *apiErr.Metadata == nil {
		return 0, false
	}

	resetValue, exists := (*apiErr.Metadata)[rateLimitResetKey]
	if !exists {
		return 0, false
	}

	var ms float64
	switch v := resetValue.(type) {
	case float64:
		ms = v
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, false
		}
		ms = parsed
	default:
		return 0, false
	}

	if ms <= 0 {
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

func deriveErrorStatus(err error) (string, int) {
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
	m.startedCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model),
	))
}

func (m *metricsRecorder) recordRequest(ctx context.Context, provider, model, outcome string, retryed bool) {
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
	m.retryCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model),
	))
}

func (m *metricsRecorder) recordLatency(ctx context.Context, provider, model, outcome string, durationSeconds float64) {
	m.latencyHistogram.Record(ctx, durationSeconds, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model),
		attribute.String("outcome", outcome),
	))
}

func (m *metricsRecorder) recordWaitTime(ctx context.Context, provider, model string, waitSeconds float64, waitSource string) {
	m.waitHistogram.Record(ctx, waitSeconds, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model),
		attribute.String("wait_source", waitSource),
	))
}

func (m *metricsRecorder) recordRateLimitExceeded(ctx context.Context, provider, model string) {
	m.rateLimitExceededCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model),
	))
}

func (m *metricsRecorder) recordError(ctx context.Context, provider, model, errorType string, httpStatus int) {
	m.errorCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model),
		attribute.String("error.type", errorType),
		attribute.Int("http.status", httpStatus),
	))
}

type streamCreator func(ctx context.Context) (*openrouter.ChatCompletionStream, error)

func (o *LLM) executeWithRetry(ctx context.Context, createStream streamCreator) (*openrouter.ChatCompletionStream, error) {
	startTime := time.Now()
	provider := "openrouter"
	model := o.model

	maxRetries := 3
	if o.retryConfig != nil {
		var errVal error
		maxRetries, errVal = o.retryConfig.MaxAttemptsValue()
		if errVal != nil {
			return nil, fmt.Errorf("max_attempts: %w", errVal)
		}
	}

	maxRateLimitWait := config.DefaultMaxRateLimitWait
	if o.retryConfig != nil {
		var errVal error
		maxRateLimitWait, errVal = o.retryConfig.MaxRateLimitWaitValue()
		if errVal != nil {
			return nil, fmt.Errorf("max_rate_limit_wait: %w", errVal)
		}
	}

	backoffMgr, errVal := o.getBackoff()
	if errVal != nil {
		return nil, fmt.Errorf("backoff config: %w", errVal)
	}

	if o.metrics != nil {
		o.metrics.recordStarted(ctx, provider, model)
	}

	var attempt int
	var stream *openrouter.ChatCompletionStream
	var err error

	for attempt = 0; attempt < maxRetries; attempt++ {
		stream, err = createStream(ctx)
		if err == nil {
			break
		}

		if !isRetryableError(err) {
			duration := time.Since(startTime).Seconds()
			if o.metrics != nil {
				errorType, httpStatus := deriveErrorStatus(err)
				o.metrics.recordError(ctx, provider, model, errorType, httpStatus)
				o.metrics.recordRequest(ctx, provider, model, "error", attempt > 0)
				o.metrics.recordLatency(ctx, provider, model, "error", duration)
			}
			return nil, err
		}

		resetWait, hasResetWait := extractRateLimitReset(err)
		if hasResetWait {
			if resetWait <= maxRateLimitWait {
				select {
				case <-ctx.Done():
					duration := time.Since(startTime).Seconds()
					if o.metrics != nil {
						o.metrics.recordRequest(ctx, provider, model, "error", true)
						o.metrics.recordLatency(ctx, provider, model, "error", duration)
					}
					return nil, ctx.Err()
				case <-time.After(resetWait):
				}

				if o.metrics != nil {
					o.metrics.recordWaitTime(ctx, provider, model, resetWait.Seconds(), "server_reset")
					o.metrics.recordRetry(ctx, provider, model)
				}
				continue
			}

			if o.metrics != nil {
				o.metrics.recordRateLimitExceeded(ctx, provider, model)
			}
			return nil, fmt.Errorf("rate limit reset time %v exceeds configured max_wait %v; consider increasing max_rate_limit_wait in your retry block: %w", resetWait, maxRateLimitWait, err)
		}

		waitTime := backoffMgr.NextBackOff()
		if waitTime == backoff.Stop {
			break
		}

		select {
		case <-ctx.Done():
			duration := time.Since(startTime).Seconds()
			if o.metrics != nil {
				o.metrics.recordRequest(ctx, provider, model, "error", true)
				o.metrics.recordLatency(ctx, provider, model, "error", duration)
			}
			return nil, ctx.Err()
		case <-time.After(waitTime):
		}

		if o.metrics != nil {
			o.metrics.recordWaitTime(ctx, provider, model, waitTime.Seconds(), "exponential_backoff")
			o.metrics.recordRetry(ctx, provider, model)
		}
	}

	duration := time.Since(startTime).Seconds()
	if o.metrics != nil {
		if err != nil {
			errorType, httpStatus := deriveErrorStatus(err)
			o.metrics.recordError(ctx, provider, model, errorType, httpStatus)
			o.metrics.recordRequest(ctx, provider, model, "error", attempt > 0)
			o.metrics.recordLatency(ctx, provider, model, "error", duration)
		} else {
			o.metrics.recordRequest(ctx, provider, model, "success", attempt > 0)
			o.metrics.recordLatency(ctx, provider, model, "success", duration)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("retry exhausted after %d attempts: %w", attempt+1, err)
	}

	return stream, nil
}
