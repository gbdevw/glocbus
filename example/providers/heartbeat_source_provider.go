package providers

import (
	"context"
	"log"

	"github.com/gbdevw/glocbus"
	"github.com/gbdevw/glocbus/example/configuration"
	"github.com/gbdevw/glocbus/example/sources"
	"github.com/gbdevw/glocbus/propagators"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
)

func ProvideEventSource(
	lc fx.Lifecycle,
	bus glocbus.EventBusInterface,
	tracerProvider trace.TracerProvider,
	logger *log.Logger,
) *sources.HeartbeatEventSource {
	// Build the event source
	source := sources.NewHeartbeatEventSource(tracerProvider, logger)
	// Add start/stop hooks
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Register source to event bus and ask the bus to start the source
			return bus.RegisterEventSource(
				ctx,
				configuration.HeartbeatSourceId,
				source,
				propagators.NewEventPropagator(logger, tracerProvider),
				true,
				nil)
		},
		OnStop: func(ctx context.Context) error {
			return source.Stop(ctx)
		},
	})
	// Return source
	return source
}
