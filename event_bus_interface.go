package glocbus

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/gbdevw/glocbus/propagators"
)

// Can be used by the users to provide more information about an event source
type EventSourceDescription any

// Structure which holds all data and components used by the event bus to an manage event source.
type EventSourceInformation struct {
	// ID for the source of events
	Id string
	// Source of events
	Source EventSourceInterface
	// Propagator used to propagate events from the source to subscribers
	Propagator propagators.EventPropagatorInterface
	// OPtional, user defined additional information about the event source.
	Description EventSourceDescription
	// Internal channel used to push new subscribers to propagator
	newSubscribers chan chan cloudevents.Event
}

// Interface for an event bus that coordinates event sources, event propagators and subscribers.
type EventBusInterface interface {
	// # Description
	//
	// Register a new source of events, bind it to the provided propagator and start the
	// propagator. Optionally, the event source will be started after the propagator.
	//
	// # Inputs
	//
	//	- id: Unique ID for the event source.
	//	- source: Event source to register. The source can be already started when provided.
	//	- propagator: Propagator to use to propagate events to subscribers. Must not be started.
	//    A separate propagator is expected to be used for each source.
	//	- startSource: Indicates whether source should be started after starting the propagator.
	//	- description: Optional user defined struct used to provide additional information about
	//    the event source. Can be nil.
	//
	// # Return
	//
	// An error if the event source could not be registered. Possible causes are:
	//	- The provided ID is not unique
	//	- The provided propagator is already started.
	//	- In case startSource is true, the provided source fails to start.
	RegisterEventSource(
		ctx context.Context,
		id string,
		source EventSourceInterface,
		propagator propagators.EventPropagatorInterface,
		startSource bool,
		description EventSourceDescription,
	) error
	// # Description
	//
	// Subscribe to a source of event identified by the provided ID. The provided channel will be
	// used by the propagator to publish events from the event source.
	//
	// # Implementation requirements & hints
	//
	//	- The channel used to push new subscribers to propagator can be closed by the propagator
	//    when it is stopped or when the event source is stopped. The event bus must handle the
	//    case when it writes to a closed channel. In this case, the event source must be removed
	//    from the list of event sources and an error must be returned to subscriber.
	//
	//	- The choice to use blocking or non-blocking write to propagate events is left to the
	//    propagator. The same thing applies to scaling: It is up to the propagator implementation
	//    to make clear statements about how it scales, handles congestion, ...
	//
	// # Inputs
	//
	//	- ctx: Context used for tracing purpose.
	//	- id: ID of the event source
	//	- name: Name defined by the subscriber to identify itself. It is not required to be unique.
	//	- subscriber: Channel provided by the subscriber to receive events from the source.
	//
	// # Return
	//
	// An error when subscription failed. Possible causes are:
	//	- The event source does not exist or has been removed (missing ID).
	//	- The propagator has closed the channel to receive new subscriptions.
	SubscribeEventSource(
		ctx context.Context,
		id string,
		name string,
		subscriber chan cloudevents.Event,
	) error
	// # Description
	//
	// List all currently available event sources.
	//
	// # Return
	//
	// The list of currently available event sources.
	ListEventSources() []EventSourceInformation
}
