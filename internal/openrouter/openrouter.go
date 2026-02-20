package openrouter

import (
	"net/http"

	openrouter "github.com/revrost/go-openrouter"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

var tracer = otel.Tracer("openrouter")

type LLM struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *openrouter.Client
}

func NewLLM(apiKey, baseURL, model string) *LLM {
	if baseURL == "" {
		baseURL = defaultOpenRouterBaseURL
	}

	config := openrouter.DefaultConfig(apiKey)
	config.BaseURL = baseURL
	config.HttpReferer = "https://github.com/meschbach/marvin"
	config.XTitle = "Marvin"

	config.HTTPClient = &http.Client{
		Transport: otelhttp.NewTransport(
			http.DefaultTransport,
			otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator()),
		),
	}

	client := openrouter.NewClientWithConfig(*config)

	return &LLM{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: client,
	}
}
