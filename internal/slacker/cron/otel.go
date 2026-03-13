package cron

import "go.opentelemetry.io/otel"

var tracer = otel.Tracer("github.com/meschbach/marvin/internal/slacker/cron")
