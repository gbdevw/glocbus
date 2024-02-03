package glocbus

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/gbdevw/glocbus/propagators"
	"github.com/gbdevw/glocbus/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Event-bus implementation that amanges event sources and propagators to relay events in a
// publish-subscribe fashion.
type Glocbus struct {
	// Mutex used to protect the sources of events
	mu sync.Mutex
	// Registered sources of events
	sources map[string]*EventSourceInformation
	// Tracer used to instrument code
	tracer trace.Tracer
}

// # Description
//
// Build and return a new event bus without any event source set.
//
// # Inputs
//
//   - tracerProvider: Tracer provider ot use to get the tracer used to instrument code. If nil,
//     the global tracer provider will be used.
//
// # Return
//
// A new Glocbus without any source.
func NewGlocbus(tracerProvider trace.TracerProvider) *Glocbus {
	if tracerProvider == nil {
		tracerProvider = otel.GetTracerProvider()
	}
	return &Glocbus{
		mu:      sync.Mutex{},
		sources: map[string]*EventSourceInformation{},
		tracer:  tracerProvider.Tracer(tracing.PackageName, trace.WithInstrumentationVersion(tracing.PackageVersion)),
	}
}

// # Description
//
// Register a new source of events, bind it to the provided propagator and start the
// propagator. Optionally, the event source will be started after the propagator.
//
// # Inputs
//
//   - ctx: Context used for tracing purpose.
//   - id: Unique ID for the event source.
//   - source: Event source to register. The source can be already started when provided.
//   - propagator: Propagator to use to propagate events to subscribers. Must not be started.
//     A separate propagator is expected to be used for each source.
//   - startSource: Indicates whether source should be started after starting the propagator.
//   - description: Optional user defined struct used to provide additional information about
//     the event source. Can be nil.
//
// # Return
//
// An error if the event source could not be registered. Possible causes are:
//   - The provided ID is not unique
//   - The provided propagator is already started.
//   - In case startSource is true, the provided source fails to start.
func (bus *Glocbus) RegisterEventSource(
	ctx context.Context,
	id string,
	source EventSourceInterface,
	propagator propagators.EventPropagatorInterface,
	startSource bool,
	description EventSourceDescription,
) error {
	// Start span
	ctx, span := bus.tracer.Start(ctx, "register_event_source",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("id", id),
			attribute.Bool("start_source", startSource),
		))
	defer span.End()
	select {
	case <-ctx.Done():
		// Shortcut method: provided context has epxired
		return tracing.HandleAndTraceErr(span, fmt.Errorf("failed to register event source: %w", ctx.Err()))
	default:
		// Validate inputs
		if source == nil || propagator == nil {
			return tracing.HandleAndTraceErr(span, fmt.Errorf("failed to register event source: event source and propagator must not be nil"))
		}
		// Lock sources mutex
		bus.mu.Lock()
		defer bus.mu.Unlock()
		// Check if source is not registered yet
		if bus.sources[id] != nil {
			return tracing.HandleAndTraceErr(span, fmt.Errorf("failed to register event source: an event source with the id %s is already registered", id))
		}
		// Build event source informations
		esi := &EventSourceInformation{
			Id:             id,
			Source:         source,
			Propagator:     propagator,
			Description:    description,
			newSubscribers: make(chan propagators.Subscription),
			confirmations:  make(chan error),
		}
		// Start the propagator
		err := propagator.Start(ctx, id, source.GetChannel(), esi.newSubscribers, esi.confirmations)
		if err != nil {
			return tracing.HandleAndTraceErr(span, fmt.Errorf("failed to register event source: failed to start the propagator: %w", err))
		}
		// If instructed, start the source
		if startSource {
			err = source.Start(ctx)
			if err != nil {
				// Trace start error and stop the propagator
				defer tracing.HandleAndTraceErr(span, propagator.Stop(ctx))
				return tracing.HandleAndTraceErr(span, fmt.Errorf("failed to register event source: failed to start the source: %w", err))
			}
		}
		// Add the new source to the map of registered sources and exit
		bus.sources[id] = esi
		span.SetStatus(codes.Ok, codes.Ok.String())
		return nil
	}
}

// # Description
//
// Subscribe to a source of event identified by the provided ID. The provided channel will be
// used by the propagator to publish events from the event source.
//
// # Implementation requirements & hints
//
//   - The channel used to push new subscribers to propagator can be closed by the propagator
//     when it is stopped or when the event source is stopped. The event bus must handle the
//     case when it writes to a closed channel. In this case, the event source must be removed
//     from the list of event sources and an error must be returned to subscriber.
//
//   - The choice to use blocking or non-blocking write to propagate events is left to the
//     propagator. The same thing applies to scaling: It is up to the propagator implementation
//     to make clear statements about how it scales, handles congestion, ...
//
// # Inputs
//
//   - ctx: Context used for stracing purpose.
//   - id: ID of the event source
//   - name: Name defined by the subscriber to identify itself. It is not required to be unique.
//   - subscriber: Channel provided by the subscriber to receive events from the source.
//
// # Return
//
// An error when subscription failed. Possible causes are:
//   - The event source does not exist
//   - The event source has been closed.
//   - The provided context has expired before subscription is complete.
//
// In the two later case, the method will close the provided channel.
func (bus *Glocbus) SubscribeEventSource(
	ctx context.Context,
	id string,
	name string,
	subscriber chan event.Event,
) error {
	// Start span
	ctx, span := bus.tracer.Start(ctx, "subscribe_event_source", trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("id", id),
			attribute.String("name", name),
		))
	defer span.End()
	select {
	case <-ctx.Done():
		// Shortcut method: provided context has epxired
		return tracing.HandleAndTraceErr(span, fmt.Errorf("failed to subscribe to event source %s: %w", id, ctx.Err()))
	default:
		// Validate inputs
		if subscriber == nil {
			return tracing.HandleAndTraceErr(span, fmt.Errorf("failed to subscribe to event source %s: provided subscriber channel is nil", id))
		}
		// Lock sources mutex
		bus.mu.Lock()
		defer bus.mu.Unlock()
		// Fetch the target event source
		esi := bus.sources[id]
		if esi == nil {
			return tracing.HandleAndTraceErr(span, fmt.Errorf("failed to subscribe to event source: no event source is registrered for id: %s", id))
		}
		// Send new subscription to propagator by using the dedicated channel
		esi.newSubscribers <- propagators.Subscription{Name: name, Subscriber: subscriber}
		// Wait for the results
		select {
		case confirmation, more := <-esi.confirmations:
			// Check if channel is closed -> propagator closed it because source has been closed
			if !more {
				// Close the subscriber channel
				close(subscriber)
				// Remove the event source
				delete(bus.sources, id)
				// Return an error
				return tracing.HandleAndTraceErr(span, fmt.Errorf("failed to subscribe to event source %s: event source has been closed", id))
			}
			// Check confirmation
			if confirmation != nil {
				// Subscription failed - return the received error
				return tracing.HandleAndTraceErr(span, fmt.Errorf("failed to subscribe to event source %s: %w", id, confirmation))
			}
			// Exit - success
			span.SetStatus(codes.Ok, codes.Ok.String())
			return nil
		case <-ctx.Done():
			// Provided context has expired, close the subscriber channel provided to propagator
			// so it will remove subscriber and then return an error
			close(subscriber)
			return tracing.HandleAndTraceErr(span, fmt.Errorf("failed to subscribe to event source %s: %w", id, ctx.Err()))
		}
	}
}

// # Description
//
// List all currently available event sources.
//
// # Return
//
// The list of currently available event sources.
func (bus *Glocbus) ListEventSources() []EventSourceInformation {
	// Lock sources mutex
	bus.mu.Lock()
	defer bus.mu.Unlock()
	// Put sources info into an array
	results := make([]EventSourceInformation, len(bus.sources))
	index := 0
	for _, v := range bus.sources {
		results[index] = *v
		index = index + 1
	}
	return results
}
