package glocbus

import (
	"context"

	"github.com/cloudevents/sdk-go/v2/event"
)

// Interface for an event source.
type EventSourceInterface interface {
	// # Description
	//
	// Provide the channel used by the event source to publish its events.
	//
	// # Implementation requirements & hints
	//
	//	- The method must return the same channel instance to all callers.
	//	- The source must provide its publication channel even if it has not been started yet.
	//	- The source must continue providing the same channel even if the source has been stopped.
	//	- To achieve end to end event traceability, events are expected to embed tracing data as
	//    described in the following documentation:
	//    https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/extensions/distributed-tracing.md
	//
	// # Return
	//
	// The channel used by this source to publish its events.
	GetChannel() chan event.Event
	// # Description
	//
	// Start the event source that will start publishing events on its publication channel.
	//
	// # Implementation requirements & hints
	//
	//	- The method must return an error if the source has already been started.
	//	- The method must return an error if the provided context has been canceled or has expired.
	//	- The method must return an error if the source has been stopped (no restart).
	//	- The method must close the publication channel.
	//
	// # Inputs
	//
	//	- ctx: Context used for tracing purpose.
	//
	// # Return
	//
	// An error if the event source could not be started.
	Start(ctx context.Context) error
	// # Description
	//
	// Stop the event source and close the publication channel.
	//
	// # Implementation requirements & hints
	//
	//	- The method must return an error if the source has not been started.
	//	- The method must return an error if the source has already been stopped.
	//	- The method must close the publication channel even if the underlying event source could
	//    not properly closed.
	//	- The closed publication channel must be kept: source must not be restarted (stale source).
	//
	// # Inputs
	//
	//	- ctx: Context used for tracing purpose.
	//
	// # Return
	//
	// An error if the event source could not be closed.
	Stop(ctx context.Context) error
}
