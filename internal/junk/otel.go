package junk

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RecordSpanError records an error to the given span.  This will note the specific span as an Error span.
func RecordSpanError(span trace.Span, err error) error {
	RecordSpanErrorNoLint(span, err)
	return err
}

// RecordSpanErrorNoLint records the error to the span but does not return the error.  This is in cases where we might
// do other things but should still record the error.
func RecordSpanErrorNoLint(span trace.Span, err error) {
	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)
	span.AddEvent("error", trace.WithAttributes(attribute.String("error", err.Error())))
}

// MaybeRecordSpanError records an error if err is not nil
func MaybeRecordSpanError(span trace.Span, err error) error {
	if err == nil {
		return nil
	}
	return RecordSpanError(span, err)
}
