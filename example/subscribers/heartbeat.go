package subscribers

import (
	"context"
	"fmt"
	"io"
	"log"

	otelObs "github.com/cloudevents/sdk-go/observability/opentelemetry/v2/client"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/gbdevw/glocbus"
	"github.com/gbdevw/glocbus/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Struct for a subscriber that will listen to heartbeats
type HeartbeatSubscriber struct {
	// ID of the event source to subscribe to
	sourceId string
	// Subscriber name
	name string
	// Channel used to receive published events
	subscriber chan event.Event
	// Event bus where events are published
	bus glocbus.EventBusInterface
	// Function used to stop internal goroutine
	stop context.CancelFunc
	// Tracer used to instrument code
	tracer trace.Tracer
	// Logger used to publish debug logs
	logger *log.Logger
}

// # Description
//
// # Build and return a non-started heartbeat subscriber.
//
// # Inputs
//
//   - bus: Bus which is used to publish events to this subscriber will subscribe to.
//   - sourceId: ID of the source to subscribe to.
//   - name: Name of the subscriber
//   - tracerProvider: Provider to use to get a tracer. If nil, global tracer provider is used.
//   - logger: Logger to use to publish debug logs. If nil, debug logs will be discarded.
//
// # Return
//
// A new, non-started heartbeat subscriber.
func NewHeartbeatSubscriber(bus glocbus.EventBusInterface, sourceId string, name string, tracerProvider trace.TracerProvider, logger *log.Logger) *HeartbeatSubscriber {
	if logger == nil {
		// Use a logger which does not print logs the provided one is nil
		logger = log.New(io.Discard, "", log.Flags())
	}
	if tracerProvider == nil {
		// Use global tracer provider if provided one is nil
		tracerProvider = otel.GetTracerProvider()
	}
	// Build and return the event source
	return &HeartbeatSubscriber{
		sourceId:   sourceId,
		name:       name,
		subscriber: make(chan event.Event, 10),
		bus:        bus,
		stop:       nil,
		tracer:     tracerProvider.Tracer(tracing.PackageName+".subscriber.heartbeat", trace.WithInstrumentationVersion(tracing.PackageVersion)),
		logger:     logger,
	}
}

// # Description
//
// Start the subscriber: The susbcriber will subscribe to the source of events through the event bus
// and will start an internal goroutine to process events
func (subscriber *HeartbeatSubscriber) Start(ctx context.Context) error {
	// Tracing: start span
	_, span := subscriber.tracer.Start(ctx, "start", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.SetStatus(codes.Ok, codes.Ok.String())
	defer span.End()
	// Subscribe to event source
	err := subscriber.bus.SubscribeEventSource(ctx, subscriber.sourceId, subscriber.name, subscriber.subscriber)
	if err != nil {
		return tracing.HandleAndTraLogErr(span, subscriber.logger, fmt.Errorf("[SUBSCRIBER][HEARTBEAT][%s]: failed to start subscriber: %w", subscriber.name, err))
	}
	// Create a cancellable parent context for the internal goroutine
	pctx := context.Background()
	pctx, subscriber.stop = context.WithCancel(pctx)
	// Start the internal goroutine and exit
	go processEvents(pctx, subscriber.name, subscriber.subscriber, subscriber.tracer, subscriber.logger)
	return nil
}

// # Description
//
// Stop the subscriber that will unsubscribe from the event source
func (subscriber *HeartbeatSubscriber) Stop(ctx context.Context) error {
	// Tracing: start span
	_, span := subscriber.tracer.Start(ctx, "stop", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.SetStatus(codes.Ok, codes.Ok.String())
	defer span.End()
	// Call stop method to stop internal goroutine and exit
	subscriber.stop()
	return nil
}

// # Description
//
// Continuously process incoming events from the event source until a cancellation signal is
// received from the parent context.
//
// # Inputs
//
//   - pctx: Parent context used to receive cancellation signals
//   - subscriber: Channel used to receive events from the event bus.
//   - tracer: Tracer used to instrument code
//   - logger: Logger used to publish debug logs
func processEvents(pctx context.Context, name string, subscriber chan event.Event, tracer trace.Tracer, logger *log.Logger) {
	for {
		// Tracing: start span
		ctx, span := tracer.Start(pctx, "process_events", trace.WithSpanKind(trace.SpanKindInternal))
		select {
		case <-pctx.Done():
			// Cancellation signal received - close subscriber & - exit
			//close(subscriber) - removed because panic when channel is closed twice
			span.AddEvent("exit", trace.WithAttributes(attribute.String("reason", "cancellation signal received")))
			logger.Printf("[SUBSCRIBER][HEARTBEAT][%s]: Cancellation signal received. Subscriber will be closed", name)
			span.SetStatus(codes.Ok, codes.Ok.String())
			span.End()
			return
		case event, more := <-subscriber:
			if !more {
				// Event source has been closed - close subscriber & exit
				//close(subscriber)  - removed because panic when channel is closed twice
				span.AddEvent("exit", trace.WithAttributes(attribute.String("reason", "event source has been closed")))
				logger.Printf("[SUBSCRIBER][HEARTBEAT][%s]: Event source has been closed. Subscriber will be closed", name)
				span.SetStatus(codes.Ok, codes.Ok.String())
				span.End()
				return
			}
			// Extract tracing context from event
			ectx := otelObs.ExtractDistributedTracingExtension(ctx, event)
			// Continue e2e span by extracting trace context from event
			_, espan := tracer.Start(ectx, "process_heartbeat", trace.WithSpanKind(trace.SpanKindConsumer),
				trace.WithAttributes(
					attribute.String("subscription_name", name),
					attribute.String("id", event.ID()),
					attribute.String("type", event.Type()),
					attribute.String("source", event.Source()),
				))
			// Log heartbeat
			logger.Printf("[SUBSCRIBER][HEARTBEAT][%s]: event received: '%s'", name, string(event.Data()))
			// Close spans and loop
			espan.SetStatus(codes.Ok, codes.Ok.String())
			espan.End()
			span.SetStatus(codes.Ok, codes.Ok.String())
			span.End()
		}
	}
}
