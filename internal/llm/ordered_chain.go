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
			if metrics != nil {
				metrics.recordCircuitEvent(chain.ctx, modelName,
					spanStateToString(int(from)), spanStateToString(int(to)))
			}
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
		remaining := len(c.models) - i

		modelCtx, cancel := c.withPerModelTimeout(ctx, remaining)

		tryCtx, trySpan := tracer.Start(modelCtx, "OrderedChain.tryModel",
			trace.WithAttributes(
				attribute.String("model.label", entry.Label),
				attribute.String("model.provider", entry.Provider),
				attribute.Int("attempt.index", i),
			),
		)

		resp, err := entry.Breaker.Execute(tryCtx, func(innerCtx context.Context) (*ChatResponse, error) {
			var finalResp *ChatResponse
			callErr := entry.LLM.Chat(innerCtx, req, func(ctx context.Context, resp *ChatResponse) error {
				finalResp = resp
				return onResponse(ctx, resp)
			})
			return finalResp, callErr
		})
		trySpan.End()
		cancel()

		if err == nil {
			if c.metrics != nil {
				c.metrics.recordTryCount(ctx, int64(attempted))
				c.metrics.recordLatency(ctx, "success", time.Since(startTime).Seconds())
				if attempted == 1 {
					c.metrics.recordResult(ctx, "success")
				} else {
					c.metrics.recordResult(ctx, "recovered")
				}
			}
			span.SetAttributes(
				attribute.String("chain.result", c.chainResult(attempted)),
				attribute.Int("models.attempted", attempted),
				attribute.String("chain.final_model", entry.Label),
			)
			span.SetStatus(codes.Ok, "")
			_ = resp
			return nil
		}

		if c.metrics != nil && errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			c.metrics.recordTimeout(ctx, entry.Label, defaultPerModelTimeout.Seconds())
		}

		lastAttemptedModel = entry.Label

		if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrNoToken) {
			lastErr = err
			continue
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			if ctx.Err() != nil {
				span.RecordError(ctx.Err())
				span.SetStatus(codes.Error, "context canceled")
				return err
			}
			lastErr = err
			continue
		}

		lastErr = err

		if i+1 < len(c.models) && c.accessCheck != nil {
			for j := i + 1; j < len(c.models); j++ {
				nextEntry := &c.models[j]
				if c.accessCheck == nil || c.accessCheck(nextEntry.Label) {
					if c.metrics != nil {
						c.metrics.recordSwitch(ctx, entry.Label, nextEntry.Label)
					}
					break
				}
			}
		} else if i+1 < len(c.models) {
			nextEntry := &c.models[i+1]
			if c.metrics != nil {
				c.metrics.recordSwitch(ctx, entry.Label, nextEntry.Label)
			}
		}
	}

	if attempted == 0 {
		span.SetAttributes(
			attribute.String("chain.result", "exhausted"),
			attribute.Int("models.attempted", 0),
		)
		span.SetStatus(codes.Error, "no models available")
		if c.metrics != nil {
			c.metrics.recordResult(ctx, "exhausted")
			c.metrics.recordLatency(ctx, "exhausted", time.Since(startTime).Seconds())
		}
		return fmt.Errorf("%w: no models available (all denied by access check)", ErrAllModelsExhausted)
	}

	span.SetAttributes(
		attribute.String("chain.result", "exhausted"),
		attribute.Int("models.attempted", attempted),
		attribute.String("chain.final_model", lastAttemptedModel),
	)
	span.RecordError(lastErr)
	span.SetStatus(codes.Error, "all models exhausted")

	if c.metrics != nil {
		c.metrics.recordTryCount(ctx, int64(attempted))
		c.metrics.recordResult(ctx, "exhausted")
		c.metrics.recordLatency(ctx, "exhausted", time.Since(startTime).Seconds())
	}

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
