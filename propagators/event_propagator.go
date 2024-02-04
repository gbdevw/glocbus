package propagators

import (
	"context"
	"fmt"
	"io"
	"log"

	otelObs "github.com/cloudevents/sdk-go/observability/opentelemetry/v2/client"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/gbdevw/glocbus/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// Capacity used by internal relay channels
	internal_relay_channels_capacity = 5
)

// Event propagator that will propagate events from a source to subscribers.
//
// The event propagator will use one dedicated goroutine to relay events to each subscriber. The
// propagator will use non-blocking writes and events will be discarded when the subscriber's
// channel is full.
//
// The event propagator does not support multiple event sources.
type EventPropagator struct {
	// ID for the source bound to the propagator
	sourceId string
	// Source channel
	source chan event.Event
	// Channel used to listen for new subscriptions
	newSubscribers chan Subscription
	// Channel used to publish the outcome of the subscription (nil or error)
	confirmations chan error
	// Channel used to receive notifications about unsubscribe
	removedSubscribers chan int64
	// Subscribers
	subscribers map[int64]Subscription
	// Ever increasing counter used to assign internal IDs to subscribers.
	counter int64
	// Cancel function used to stop the internal goroutine
	stop context.CancelFunc
	// Tracer used to instrument code
	tracer trace.Tracer
	// Logger used to publish debug logs
	logger *log.Logger
}

// # Description
//
// Create a new, non-started event propagator.
//
// # Inputs
//
//   - logger: Logger to use to publish debug logs. If nil, a logger with a io.Discard is used.
//
//   - tracerProvider: tracer provider to use to get a tracer to instrument code. If nil, global
//     tracer provider will be used.
//
// # Return
//
// A new, not started event propagator.
func NewEventPropagator(logger *log.Logger, tracerProvider trace.TracerProvider) *EventPropagator {
	if logger == nil {
		// Use a logger with io.Discard as sink
		logger = log.New(io.Discard, "", log.Default().Flags())
	}
	if tracerProvider == nil {
		// Use global tracer provider
		tracerProvider = otel.GetTracerProvider()
	}
	return &EventPropagator{
		source:             nil, // Be sure to set to nil
		newSubscribers:     nil, // Be sure to set to nil
		removedSubscribers: make(chan int64),
		subscribers:        map[int64]Subscription{},
		counter:            int64(0),
		stop:               nil, // Be sure to set to nil
		tracer:             tracerProvider.Tracer(tracing.PackageName+".event_propagator", trace.WithInstrumentationVersion(tracing.PackageVersion)),
		logger:             logger,
	}
}

// # Description
//
// Start the propagator. The propagator must start and manage one or several goroutines that
// will listen for new subscribers and for events to propagate to subscribers.
//
// # Inputs
//
//   - ctx: Context used for tracing purpose.
//   - sourceId: ID of the event source.
//   - source: Source of events to propagate to subscribers.
//   - newSubscribers: Channel used to listen for new subscriptions.
//   - confirmations: Channel used by the propagator to publish the outcome of the subscription.
//
// # Implementation requirements & hints
//
//   - The propagator should be used to propagate event from only one event source.
//   - Do not use the provided context to receive cancellation signals as the provided context
//     can expire a bit after Start has been called.
//   - Subscribers' names are not required to be unique.
//   - Subscriber's channel can be closed by the subscriber to unsubscribe. Propagator must
//     handle the case when it writes to a closed channel. Once a subscriber's channel is
//     closed, it must be removed from the list of subscribers to which events are propagated to.
//   - The source channel can be closed in case the event source is stopped. In this case, the
//     propagator must close all subscriber's channel and stop all internal goroutines.
//   - The propagator must return an error if it has already been started.
//   - The propagator must return an error if it has been stopped (no restart - stale source).
//   - When stopping, the propagator must close the confirmations channel so the event bus can
//     reject new subscriptions and remove the event source.
//
// # Return
//
// An error if propagator failed to start.
func (propagator *EventPropagator) Start(ctx context.Context, sourceId string, source chan event.Event, newSubscribers chan Subscription, confirmations chan error) error {
	// Tracing: Start span
	ctx, span := propagator.tracer.Start(ctx, "start",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String("source_id", sourceId)))
	defer span.End()
	propagator.logger.Printf("starting propagator for source %s", sourceId)
	select {
	case <-ctx.Done():
		// Shortcut method if provided context has expired
		return tracing.HandleAndTraLogErr(span, propagator.logger, fmt.Errorf("failed to start propagator : %w", ctx.Err()))
	default:
		// Validate inputs
		if source == nil || newSubscribers == nil || confirmations == nil {
			return tracing.HandleAndTraLogErr(span, propagator.logger, fmt.Errorf("failed to start propagator : source and subscription channels must not be nil"))
		}
		// Check if not started yet
		if propagator.stop != nil {
			return tracing.HandleAndTraLogErr(span, propagator.logger, fmt.Errorf("failed to start propagator : propagator cannot be restarted"))
		}
		// Create cancelable context that will be used by internal goroutine to catch stop signal
		// Start from a fresh context as provided context might expire.
		ctx, propagator.stop = context.WithCancel(context.Background())
		propagator.newSubscribers = newSubscribers
		propagator.confirmations = confirmations
		propagator.source = source
		propagator.sourceId = sourceId
		// Start internal goroutine that will run the propagator
		go propagator.runPropagator(ctx)
		return nil
	}
}

// # Description
//
// Stop the propagator. The propagator is expected to stop listening for new subscribers or
// events, close all its current subscribers'channels, close the newSubscriber channel and
// stop all its internal goroutines.
//
// # Inputs
//
//   - ctx: Context used for tracing purpose.
//
// # Return
//
// An error if propatagor failed to stop.
func (propagator *EventPropagator) Stop(ctx context.Context) error {
	// Tracing: Start span
	_, span := propagator.tracer.Start(ctx, "stop", trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String("source_id", propagator.sourceId)))
	defer span.End()
	defer span.SetStatus(codes.Ok, codes.Ok.String())
	// Stop internal goroutine by calling cancel function and exit
	propagator.logger.Printf("[PROPAGATOR][%s]: stopping event propagator", propagator.sourceId)
	propagator.stop()
	return nil
}

// # Description
//
// This method will be run by an internal goroutine and that will continuously propagate events,
// listen for new subscriptions, removed subscriptions and cancellation signals.
//
// # Inputs
//
// - pctx: Parent context used to catch cancellation signals.
func (propagator *EventPropagator) runPropagator(pctx context.Context) {
	// Defer a function that will do the cleanup
	defer func(propagator *EventPropagator) {
		// Close confirmations channel so no new subscription will be accepted
		close(propagator.confirmations)
		// Close subscriber's channel - will stop the underlying propagator
		for k, v := range propagator.subscribers {
			propagator.logger.Printf("[PROPAGATOR][%s]: closing subscription named %s: %d", propagator.sourceId, v.Name, k)
			close(v.Subscriber)
		}
	}(propagator)
	// Loop continuously to receive events, subscriptions, ...
	for {
		// Start span
		_, span := propagator.tracer.Start(context.Background(), "run_propagator",
			trace.WithSpanKind(trace.SpanKindInternal),
			trace.WithAttributes(attribute.String("source_id", propagator.sourceId)))
		// Wait for either a cancellation signal, an event to propagate, a new subscriber or an
		// unsubscribe event
		select {
		case <-pctx.Done():
			// Parent context has been cancelled: close all propagators
			propagator.logger.Printf("[PROPAGATOR][%s]: cancellation signal received", propagator.sourceId)
			// Defer span end and exit to trigger deferred cleanup
			defer span.SetStatus(codes.Ok, codes.Ok.String())
			defer span.End()
			return
		case rm := <-propagator.removedSubscribers:
			// Remove subscriber and loop until channel is empty
			empty := false
			for !empty {
				// Close internal channel and remove subscriber
				propagator.logger.Printf("[PROPAGATOR][%s]: removing subscription %d", propagator.sourceId, rm)
				span.AddEvent("unsubscribe", trace.WithAttributes(attribute.Int64("subscription_id", rm)))
				close(propagator.subscribers[rm].Subscriber)
				delete(propagator.subscribers, rm)
				// Use a non-blocking read to watch for other unsubscribe events
				select {
				case rm = <-propagator.removedSubscribers:
					// Continue removing susbcribers
					continue
				default:
					// Exit loop
					empty = true
				}
			}
		case nws := <-propagator.newSubscribers:
			// Add new subscriber and loop until channel is empty
			empty := false
			for !empty {
				// Tracing: trace new subscription
				propagator.logger.Printf("[PROPAGATOR][%s]: new subscription named %s: %d", propagator.sourceId, nws.Name, propagator.counter)
				span.AddEvent("subscribe", trace.WithAttributes(
					attribute.Int64("subscription_id", propagator.counter),
					attribute.String("subscription_name", nws.Name)))
				// Put aside the provided subscriber's channel
				subscriberChannel := nws.Subscriber
				// Create an internal channel to relay events to a goroutine that will publish
				// events to the new subscriber. Put some capacity to prevent message discarding
				// when the goroutine is lagging a bit behind.
				internalChannel := make(chan event.Event, internal_relay_channels_capacity)
				// Start a dedicated goroutine to relay published events to the subscriber
				go propagateEvents(
					pctx, // Use the context provided when the main goroutine has started
					propagator.sourceId,
					propagator.counter, // Use the counter as id
					nws.Name,
					propagator.removedSubscribers,
					// Internal channel will be used as the source by the goroutine dedicated to the subscriber
					internalChannel,
					subscriberChannel,
					propagator.logger,
					propagator.tracer)
				// Replace the dest channel in the new subscription by the internal channel
				nws.Subscriber = internalChannel
				// Save the new subscription
				propagator.subscribers[propagator.counter] = nws
				// Send confirmation as a nil value on the confirmations channel
				propagator.confirmations <- nil
				// Increase the counter
				propagator.counter = propagator.counter + 1
				// Use a non-blocking read to watch for other new subscribers
				select {
				case nws = <-propagator.newSubscribers:
					// Loop to add new subscriber
					continue
				default:
					// Exit loop
					empty = true
				}
			}
		case event, more := <-propagator.source:
			// Check if channel is closed
			if !more {
				// Source is closed
				propagator.logger.Printf("[PROPAGATOR][%s]: event source has been closed", propagator.sourceId)
				// Defer span end and exit to trigger deferred cleanup
				defer span.SetStatus(codes.Ok, codes.Ok.String())
				defer span.End()
				return
			}
			// Extract tracing context from event and start a span from it to continue tracing
			// from producer and not break e2e tracing.
			ectx := otelObs.ExtractDistributedTracingExtension(context.Background(), event)
			// Relay a copy of the event to each subscriber
			for id, subscriber := range propagator.subscribers {
				// Start a span to trace event propagation to subscriber
				propagator.logger.Printf("[PROPAGATOR][%s]: delivering event %s to propagator %d", propagator.sourceId, event.ID(), id)
				ctx, espan := propagator.tracer.Start(ectx, "propagate_event",
					trace.WithSpanKind(trace.SpanKindInternal),
					trace.WithAttributes(
						attribute.String("source_id", propagator.sourceId),
						attribute.Int64("subscription_id", id),
						attribute.String("subscription_name", subscriber.Name),
						attribute.String("id", event.ID()),
						attribute.String("source", event.Source()),
						attribute.String("type", event.Type()),
						attribute.String("specversion", event.SpecVersion()),
					))
				// Inject span tracing context into the copy of event that will be sent to subscriber
				ce := event.Clone()
				otelObs.InjectDistributedTracingExtension(ctx, ce)
				// Use non-blocking write - discard non delivered event
				select {
				case subscriber.Subscriber <- ce:
					// Loop to next susbcriber
					espan.SetStatus(codes.Ok, codes.Ok.String())
					espan.End()
					continue
				default:
					// Discard event - record error and close event's span
					espan.RecordError(fmt.Errorf("failed to deliver event %s to propagator %d: propagator's channel is full", event.ID(), id))
					propagator.logger.Printf("[PROPAGATOR][%s]: failed to deliver event %s to propagator %d: propagator's channel is full", propagator.sourceId, event.ID(), id)
					espan.SetStatus(codes.Error, codes.Error.String())
					espan.End()
				}
			}
		}
		// End span
		span.SetStatus(codes.Ok, codes.Ok.String())
		span.End()
	}
}

// Logic for a goroutine that will continuously relay events from the provided source channel to
// the desitnation until either:
//   - A cancellation signal is received from the provided parent context
//   - The provided source channel is closed.
//   - The provided destination channel is closed.
//
// When exitting, the goroutine will close the destination channel and send a unsubscribe event to
// the propagator to be removed from the list of subscribers.
func propagateEvents(
	pctx context.Context,
	sourceId string,
	id int64,
	name string,
	unsubscribe chan int64,
	source chan event.Event,
	dest chan event.Event,
	logger *log.Logger,
	tracer trace.Tracer,
) {
	// Pointers of pointer to retain current span and event. Will be used by the closure to close
	// the ongoing span when goroutine panics because it writes to a closed channel.
	currSpan := new(trace.Span)
	currEvent := new(*event.Event)
	// Defer a closure that will do the cleanup
	defer func() {
		if *currSpan != nil && *currEvent != nil {
			// Close ongoing span and record event about discarded event
			(*currSpan).AddEvent("discarded_event", trace.WithAttributes(
				attribute.String("reason", "subscriber's channel has been closed"),
				attribute.Int64("subscription_id", id),
				attribute.String("subscription_name", name),
				attribute.String("id", (**currEvent).ID()),
				attribute.String("source", (**currEvent).Source()),
				attribute.String("type", (**currEvent).Type()),
				attribute.String("specversion", (**currEvent).SpecVersion()),
			))
			// Not an error, subscriber can close it channel to unsbubscribe
			(*currSpan).SetStatus(codes.Ok, codes.Ok.String())
			(*currSpan).End()
		}
		// Notify unsubscribe event
		unsubscribe <- id
		if recover() == nil {
			// Exit is not caused by write on closed dest channel close destination channel.
			// Defer a function that will ignore panic that can be caused if the channel is closed
			// a seocnd time. Propagator must ensure it is closed.
			defer func() { recover() }()
			close(dest)
		} else {
			logger.Printf("[PROPAGATOR][%s][%d][%s]: destination channel has been closed by the subscriber. Exiting", sourceId, id, name)
		}
	}()
	// Continuously propagate events
	for {
		select {
		case <-pctx.Done():
			// Cancellation signal has been received. Exit and let cleanup happen
			logger.Printf("[PROPAGATOR][%s][%d][%s]: cancellation signal received. Exiting", sourceId, id, name)
			return
		case event, more := <-source:
			if !more {
				// source channel has been closed - exit and let cleanup happen
				logger.Printf("[PROPAGATOR][%s][%d][%s]: source channel has been closed. Exiting", sourceId, id, name)
				return
			}
			// Tracing: extract trace id from event and continue propagator's span
			logger.Printf("[PROPAGATOR][%s][%d][%s]: propagating event %s from %s", sourceId, id, name, event.ID(), event.Source())
			ectx := otelObs.ExtractDistributedTracingExtension(context.Background(), event)
			span := trace.SpanFromContext(ectx)
			// Set currEvent and currSpan
			*currSpan = span
			*currEvent = &event
			// Use non-blocking write to propagate event. Discard if dest channel is full.
			select {
			case dest <- event:
				// Event has been delivered to susbcriber
				span.SetStatus(codes.Ok, codes.Ok.String())
				logger.Printf("[PROPAGATOR][%s][%d][%s]: event %s delivered", sourceId, id, name, event.ID())
			default:
				// Record error: delivery failed
				span.RecordError(fmt.Errorf("failed to deliver event %s to %s: subscriber's channel is full", event.ID(), name))
				logger.Printf("[PROPAGATOR][%s][%d][%s]: failed to deliver event %s: subscriber's channel is full", sourceId, id, name, event.ID())
			}
			// Close span and void receivers
			span.End()
			*currSpan = nil
			*currEvent = nil
		}
	}
}
