package openrouter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/revrost/go-openrouter"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const rateLimitStatusCode = 429

func isRetryableError(err error) bool {
	var apiErr *openrouter.APIError
	ok := errors.As(err, &apiErr)
	if !ok {
		return false
	}
	return apiErr.HTTPStatusCode == rateLimitStatusCode || (apiErr.HTTPStatusCode >= 500 && apiErr.HTTPStatusCode < 600)
}

func parseRetryAfterHeader(resp *http.Response) time.Duration {
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		return 0
	}

	if seconds, err := strconv.Atoi(retryAfter); err == nil {
		return time.Duration(seconds) * time.Second
	}

	if t, err := time.Parse(time.RFC1123, retryAfter); err == nil {
		waitTime := t.Sub(time.Now())
		if waitTime > 0 {
			return waitTime
		}
	}

	return 0
}

type metricsRecorder struct {
	requestCounter   metric.Int64Counter
	retryCounter     metric.Int64Counter
	latencyHistogram metric.Float64Histogram
	waitHistogram    metric.Float64Histogram
	errorCounter     metric.Int64Counter
}

func newMetricsRecorder(meter metric.Meter) (*metricsRecorder, error) {
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
		requestCounter:   requestCounter,
		retryCounter:     retryCounter,
		latencyHistogram: latencyHistogram,
		waitHistogram:    waitHistogram,
		errorCounter:     errorCounter,
	}, nil
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

func (m *metricsRecorder) recordWaitTime(ctx context.Context, provider, model string, waitSeconds float64) {
	m.waitHistogram.Record(ctx, waitSeconds, metric.WithAttributes(
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
		maxRetries = o.retryConfig.MaxRetriesValue()
	}

	var lastErr error
	var attempt int

	operation := func() (*openrouter.ChatCompletionStream, error) {
		attempt++
		stream, err := createStream(ctx)
		if err != nil {
			lastErr = err
			if isRetryableError(err) {
				return nil, err
			}
			return nil, backoff.Permanent(err)
		}
		return stream, nil
	}

	notify := func(err error, waitTime time.Duration) {
		if o.metrics != nil {
			if waitTime > 0 {
				o.metrics.recordWaitTime(ctx, provider, model, waitTime.Seconds())
			}
			if attempt > 1 {
				o.metrics.recordRetry(ctx, provider, model)
			}
		}
	}

	stream, err := backoff.Retry(ctx, operation, backoff.WithBackOff(o.getBackoff()), backoff.WithMaxTries(uint(maxRetries)), backoff.WithNotify(notify))

	duration := time.Since(startTime).Seconds()
	if o.metrics != nil {
		if err != nil {
			o.metrics.recordError(ctx, provider, model, "retry_exhausted", 0)
			o.metrics.recordRequest(ctx, provider, model, "error", attempt > 1)
			o.metrics.recordLatency(ctx, provider, model, "error", duration)
		} else {
			o.metrics.recordRequest(ctx, provider, model, "success", attempt > 1)
			o.metrics.recordLatency(ctx, provider, model, "success", duration)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("retry exhausted after %d attempts: %w", attempt, lastErr)
	}

	return stream, nil
}
