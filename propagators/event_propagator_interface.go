package propagators

import (
	"context"

	"github.com/cloudevents/sdk-go/v2/event"
)

// Contains everything needed to propagate events to the subscriber.
type Subscription struct {
	// Name provided by the subscriber to identify itself. Uniqueness must not be required.
	Name string
	// Channel provided by the subscriber to receive propagated events.
	Subscriber chan event.Event
}

// Interface for a component that will propagate events from a source to subscribers.
type EventPropagatorInterface interface {
	// # Description
	//
	// Start the propagator. The propagator must start and manage one or several goroutines that
	// will listen for new subscribers and for events to propagate to subscribers.
	//
	// # Inputs
	//
	//	- ctx: Context used for tracing purpose.
	//	- sourceId: ID of the event source.
	//	- source: Source of events to propagate to subscribers.
	//	- newSubscribers: Channel used to listen for new subscriptions.
	//	- confirmations: Channel used by the propagator to publish the outcome of the subscription.
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
	Start(ctx context.Context, sourceId string, source chan event.Event, newSubscribers chan Subscription, confirmations chan error) error
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
	Stop(ctx context.Context) error
}
