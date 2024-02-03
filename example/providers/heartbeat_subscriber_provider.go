package providers

import (
	"context"
	"log"

	"github.com/gbdevw/glocbus"
	"github.com/gbdevw/glocbus/example/configuration"
	"github.com/gbdevw/glocbus/example/subscribers"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
)

func ProvideHeartbeatSubscribers(
	lc fx.Lifecycle,
	bus glocbus.EventBusInterface,
	tracerProvider trace.TracerProvider,
	logger *log.Logger,
) []subscribers.HeartbeatSubscriber {
	names := []string{"subscriber_1", "subscriber_2"}
	subs := make([]subscribers.HeartbeatSubscriber, len(names))
	for _, name := range names {
		// Build a subscriber and add it to the list
		sub := subscribers.NewHeartbeatSubscriber(bus, configuration.HeartbeatSourceId, name, tracerProvider, logger)
		subs = append(subs, *sub)
		// Add lifecycle hooks
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				return sub.Start(ctx)
			},
			OnStop: func(ctx context.Context) error {
				return sub.Stop(ctx)
			},
		})
	}
	return subs
}
