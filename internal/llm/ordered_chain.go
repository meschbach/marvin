package llm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultPerModelTimeout = 30 * time.Second
)

// ModelEntry represents a single model in the chain with its breaker.
type ModelEntry struct {
	Label    string
	Provider string
	LLM      LLM
	Breaker  *RampingBreaker
}

// OrderedChain tries models in order, skipping unhealthy or denied models.
// It implements the LLM interface.
type OrderedChain struct {
	models      []ModelEntry
	accessCheck func(label string) bool
	metrics     *chainMetrics
	ctx         context.Context
}

var _ LLM = (*OrderedChain)(nil)

// NewOrderedChain creates a chain that tries models in declaration order.
// accessCheck may be nil (no filtering).
func NewOrderedChain(ctx context.Context, models []ModelEntry, accessCheck func(label string) bool) (*OrderedChain, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("ordered chain requires at least one model")
	}
	metrics, err := initChainMetrics()
	if err != nil {
		return nil, fmt.Errorf("initializing chain metrics: %w", err)
	}

	chain := &OrderedChain{
		models:      models,
		accessCheck: accessCheck,
		metrics:     metrics,
		ctx:         ctx,
	}

	for i := range models {
		models[i].Breaker.SetCircuitEventCallback(func(modelName string, from, to gobreaker.State) {
			metrics.recordCircuitEvent(chain.ctx, modelName,
				spanStateToString(int(from)), spanStateToString(int(to)))
		})
	}

	return chain, nil
}

// Chat iterates through models in order. For each model:
//  1. Check accessCheck — skip if denied
//  2. Apply per-model timeout
//  3. Call breaker.Execute — fall through on ErrRateLimited, ErrNoToken, or breaker-open
//  4. Return on first success
//  5. Return ErrAllModelsExhausted if all models fail
func (c *OrderedChain) Chat(ctx context.Context, req *ChatRequest, onResponse func(ctx context.Context, resp *ChatResponse) error) error {
	ctx, span := tracer.Start(ctx, "OrderedChain.Chat",
		trace.WithAttributes(
			attribute.String("chain.strategy", "ordered"),
			attribute.Int("models.total", len(c.models)),
			attribute.String("chain.first_model", c.models[0].Label),
		),
	)
	defer span.End()

	startTime := time.Now()
	var lastErr error
	attempted := 0
	var lastAttemptedModel string

	for i := range c.models {
		entry := &c.models[i]

		if ctx.Err() != nil {
			span.RecordError(ctx.Err())
			span.SetStatus(codes.Error, "context canceled")
			return ctx.Err()
		}

		if c.accessCheck != nil && !c.accessCheck(entry.Label) {
			continue
		}

		attempted++

		_, err := c.tryModel(ctx, entry, req, onResponse, i)
		if err == nil {
			c.recordSuccess(ctx, span, entry, attempted, startTime)
			return nil
		}

		lastAttemptedModel = entry.Label
		lastErr = err

		if !c.handleModelError(ctx, span, entry, i, err) {
			return err
		}
	}

	return c.recordExhausted(ctx, span, attempted, startTime, lastErr, lastAttemptedModel)
}

func (c *OrderedChain) tryModel(ctx context.Context, entry *ModelEntry, req *ChatRequest, onResponse func(context.Context, *ChatResponse) error, index int) (*ChatResponse, error) {
	modelCtx, cancel := c.withPerModelTimeout(ctx, len(c.models)-index)
	defer cancel()

	tryCtx, trySpan := tracer.Start(modelCtx, "OrderedChain.tryModel",
		trace.WithAttributes(
			attribute.String("model.label", entry.Label),
			attribute.String("model.provider", entry.Provider),
			attribute.Int("attempt.index", index),
		),
	)
	defer trySpan.End()

	return entry.Breaker.Execute(tryCtx, func(innerCtx context.Context) (*ChatResponse, error) {
		var finalResp *ChatResponse
		callErr := entry.LLM.Chat(innerCtx, req, func(ctx context.Context, resp *ChatResponse) error {
			finalResp = resp
			return onResponse(ctx, resp)
		})
		return finalResp, callErr
	})
}

func (c *OrderedChain) recordSuccess(ctx context.Context, span trace.Span, entry *ModelEntry, attempted int, startTime time.Time) {
	c.metrics.recordTryCount(ctx, int64(attempted))
	c.metrics.recordLatency(ctx, "success", time.Since(startTime).Seconds())
	if attempted == 1 {
		c.metrics.recordResult(ctx, "success")
	} else {
		c.metrics.recordResult(ctx, "recovered")
	}
	span.SetAttributes(
		attribute.String("chain.result", c.chainResult(attempted)),
		attribute.Int("models.attempted", attempted),
		attribute.String("chain.final_model", entry.Label),
	)
	span.SetStatus(codes.Ok, "")
}

func (c *OrderedChain) handleModelError(ctx context.Context, span trace.Span, entry *ModelEntry, index int, err error) bool {
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		c.metrics.recordTimeout(ctx, entry.Label, defaultPerModelTimeout.Seconds())
	}

	if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrNoToken) {
		return true
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if ctx.Err() != nil {
			span.RecordError(ctx.Err())
			span.SetStatus(codes.Error, "context canceled")
			return false
		}
		return true
	}

	c.recordModelSwitch(ctx, entry, index)
	return true
}

func (c *OrderedChain) recordModelSwitch(ctx context.Context, entry *ModelEntry, index int) {
	if index+1 >= len(c.models) {
		return
	}
	if c.accessCheck != nil {
		for j := index + 1; j < len(c.models); j++ {
			if c.accessCheck(c.models[j].Label) {
				c.metrics.recordSwitch(ctx, entry.Label, c.models[j].Label)
				return
			}
		}
		return
	}
	c.metrics.recordSwitch(ctx, entry.Label, c.models[index+1].Label)
}

func (c *OrderedChain) recordExhausted(ctx context.Context, span trace.Span, attempted int, startTime time.Time, lastErr error, lastAttemptedModel string) error {
	span.SetAttributes(
		attribute.String("chain.result", "exhausted"),
		attribute.Int("models.attempted", attempted),
	)
	if attempted == 0 {
		span.SetStatus(codes.Error, "no models available")
		c.metrics.recordResult(ctx, "exhausted")
		c.metrics.recordLatency(ctx, "exhausted", time.Since(startTime).Seconds())
		return fmt.Errorf("%w: no models available (all denied by access check)", ErrAllModelsExhausted)
	}

	span.SetAttributes(
		attribute.String("chain.final_model", lastAttemptedModel),
	)
	span.RecordError(lastErr)
	span.SetStatus(codes.Error, "all models exhausted")
	c.metrics.recordTryCount(ctx, int64(attempted))
	c.metrics.recordResult(ctx, "exhausted")
	c.metrics.recordLatency(ctx, "exhausted", time.Since(startTime).Seconds())

	if lastErr == nil {
		return ErrAllModelsExhausted
	}
	return fmt.Errorf("%w: %v", ErrAllModelsExhausted, lastErr)
}

func (c *OrderedChain) chainResult(attempted int) string {
	if attempted == 1 {
		return "success"
	}
	return "recovered"
}

func (c *OrderedChain) withPerModelTimeout(ctx context.Context, remainingModels int) (context.Context, context.CancelFunc) {
	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		remaining := time.Until(deadline)
		perModel := remaining / time.Duration(remainingModels)
		if perModel > defaultPerModelTimeout {
			perModel = defaultPerModelTimeout
		}
		if perModel < time.Second {
			perModel = time.Second
		}
		return context.WithTimeout(ctx, perModel)
	}
	return context.WithTimeout(ctx, defaultPerModelTimeout)
}
