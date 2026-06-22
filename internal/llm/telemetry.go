package llm

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	tracerName = "github.com/meschbach/marvin/llm/chain"
	meterName  = "github.com/meschbach/marvin/llm/chain"
)

var tracer = otel.Tracer(tracerName)

// chainMetrics holds OTel metric instruments for chain-level observability.
type chainMetrics struct {
	resultCounter       metric.Int64Counter
	latencyHistogram    metric.Float64Histogram
	tryCountHistogram   metric.Int64Histogram
	switchCounter       metric.Int64Counter
	circuitEventCounter metric.Int64Counter
	timeoutHistogram    metric.Float64Histogram
}

// newChainMetrics creates and initializes all chain-level metric instruments.
func newChainMetrics(meter metric.Meter) (*chainMetrics, error) {
	resultCounter, err := meter.Int64Counter("llm.chain.result",
		metric.WithDescription("Chain outcome: success, recovered, or exhausted"),
		metric.WithUnit("1"))
	if err != nil {
		return nil, err
	}

	latencyHistogram, err := meter.Float64Histogram("llm.chain.latency",
		metric.WithDescription("Total chain latency including model switches"),
		metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}

	tryCountHistogram, err := meter.Int64Histogram("llm.chain.try_count",
		metric.WithDescription("Number of models attempted per request"),
		metric.WithUnit("1"))
	if err != nil {
		return nil, err
	}

	switchCounter, err := meter.Int64Counter("llm.chain.switch",
		metric.WithDescription("Model switch events"),
		metric.WithUnit("1"))
	if err != nil {
		return nil, err
	}

	circuitEventCounter, err := meter.Int64Counter("llm.chain.circuit_event",
		metric.WithDescription("Circuit breaker state transitions"),
		metric.WithUnit("1"))
	if err != nil {
		return nil, err
	}

	timeoutHistogram, err := meter.Float64Histogram("llm.chain.per_model_timeout",
		metric.WithDescription("Per-model timeout durations"),
		metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}

	return &chainMetrics{
		resultCounter:       resultCounter,
		latencyHistogram:    latencyHistogram,
		tryCountHistogram:   tryCountHistogram,
		switchCounter:       switchCounter,
		circuitEventCounter: circuitEventCounter,
		timeoutHistogram:    timeoutHistogram,
	}, nil
}

// recordResult records the chain outcome.
func (m *chainMetrics) recordResult(ctx context.Context, result string) {
	m.resultCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("result", result),
	))
}

// recordLatency records the total chain latency.
func (m *chainMetrics) recordLatency(ctx context.Context, result string, seconds float64) {
	m.latencyHistogram.Record(ctx, seconds, metric.WithAttributes(
		attribute.String("result", result),
	))
}

// recordTryCount records how many models were attempted.
func (m *chainMetrics) recordTryCount(ctx context.Context, count int64) {
	m.tryCountHistogram.Record(ctx, count)
}

// recordSwitch records a model switch event.
func (m *chainMetrics) recordSwitch(ctx context.Context, fromModel, toModel string) {
	m.switchCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("from_model", fromModel),
		attribute.String("to_model", toModel),
	))
}

// recordCircuitEvent records a breaker state transition.
func (m *chainMetrics) recordCircuitEvent(ctx context.Context, model, from, to string) {
	m.circuitEventCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("model", model),
		attribute.String("from", from),
		attribute.String("to", to),
	))
}

// recordTimeout records a per-model timeout.
func (m *chainMetrics) recordTimeout(ctx context.Context, model string, seconds float64) {
	m.timeoutHistogram.Record(ctx, seconds, metric.WithAttributes(
		attribute.String("model", model),
	))
}

// initChainMetrics creates metrics using the global meter provider.
func initChainMetrics() (*chainMetrics, error) {
	meter := otel.Meter(meterName)
	return newChainMetrics(meter)
}

// spanStateToString converts gobreaker state to a string for metrics.
func spanStateToString(s int) string {
	switch s {
	case 0:
		return "closed"
	case 1:
		return "half_open"
	case 2:
		return "open"
	default:
		return "unknown"
	}
}
