// Package openrouter provides LLM bindings for using [OpenRouter](https://openrouter.ai/)
package openrouter

import (
	"fmt"
	"net/http"

	"github.com/cenkalti/backoff/v5"
	"github.com/meschbach/marvin/internal/config"
	"github.com/revrost/go-openrouter"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

type LLM struct {
	apiKey      string
	baseURL     string
	model       string
	retryConfig *config.RetryBlock
	metrics     *metricsRecorder
	httpClient  *openrouter.Client
}

func NewLLM(apiKey, baseURL, model string, retryConfig *config.RetryBlock) *LLM {
	if baseURL == "" {
		baseURL = defaultOpenRouterBaseURL
	}

	openrouterConfig := openrouter.DefaultConfig(apiKey)
	openrouterConfig.BaseURL = baseURL
	openrouterConfig.HttpReferer = "https://github.com/meschbach/marvin"
	openrouterConfig.XTitle = "Marvin"

	openrouterConfig.HTTPClient = &http.Client{
		Transport: otelhttp.NewTransport(
			http.DefaultTransport,
			otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator()),
		),
	}

	client := openrouter.NewClientWithConfig(*openrouterConfig)

	meter := otel.Meter("github.com/meschbach/marvin/openrouter")
	metrics, err := newMetricsRecorder(meter)
	if err != nil {
		metrics = nil
	}

	return &LLM{
		apiKey:      apiKey,
		baseURL:     baseURL,
		model:       model,
		retryConfig: retryConfig,
		metrics:     metrics,
		httpClient:  client,
	}
}

func (o *LLM) getBackoff() (backoff.BackOff, error) {
	if o.retryConfig == nil {
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = config.DefaultInitialInterval
		bo.MaxInterval = config.DefaultMaxInterval
		bo.RandomizationFactor = 0.25
		return bo, nil
	}
	bo := backoff.NewExponentialBackOff()
	initialInterval, err := o.retryConfig.InitialIntervalValue()
	if err != nil {
		return nil, fmt.Errorf("initial_interval: %w", err)
	}
	maxInterval, err := o.retryConfig.MaxIntervalValue()
	if err != nil {
		return nil, fmt.Errorf("max_interval: %w", err)
	}
	bo.InitialInterval = initialInterval
	bo.MaxInterval = maxInterval
	bo.RandomizationFactor = 0.25
	return bo, nil
}
