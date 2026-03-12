package openrouter

import "go.opentelemetry.io/otel"

var tracer = otel.Tracer("github.com/meschbach/marvin/openrouter")
