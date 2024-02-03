package tracing

import (
	"log"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Constants using by tracing
const (
	PackageName    = "eu.lake42.glocbus"
	PackageVersion = "0.0.0"
)

// Helper function used to trace and return the provided error.
func HandleAndTraceErr(span trace.Span, err error) error {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, codes.Error.String())
	}
	return err
}

// Helper function used to trace, log and return the provided error.
func HandleAndTraLogErr(span trace.Span, logger *log.Logger, err error) error {
	if err != nil {
		logger.Println(err.Error())
		span.RecordError(err)
		span.SetStatus(codes.Error, codes.Error.String())
	}
	return err
}
