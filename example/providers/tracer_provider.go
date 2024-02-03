package providers

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
)

func ProvideTraceProvider(lc fx.Lifecycle) (trace.TracerProvider, error) {
	// Create tracer provider
	exp, err := otlptracegrpc.New(context.Background())
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	// Register tracer provider in global tracer provider
	otel.SetTracerProvider(tp)
	// Register shutdown hook
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return tp.Shutdown(ctx)
		},
	})
	// Return tracer provider
	return tp, nil
}
