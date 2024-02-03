package sources

import (
	"context"
	"fmt"
	"io"
	"log"
	"strconv"
	"time"

	otelObs "github.com/cloudevents/sdk-go/observability/opentelemetry/v2/client"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/gbdevw/glocbus/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Data of a heartbeat
type Heartbeat struct {
	// Unix (seconds) timestamp when heartbeat has been published.
	Timestamp int64 `json:"timestamp"`
}

// Event source which emits heartbeat events
type HeartbeatEventSource struct {
	// Channel used to publish heartbeats
	publish chan event.Event
	// Function used to stop internal goroutine
	stop context.CancelFunc
	// Tracer used to instrument code
	tracer trace.Tracer
	// Logger used to publish debug logs
	logger *log.Logger
}

// # Description
//
// # Build and return a non-started heartbeat event source
//
// # Inputs
//
//   - tracerProvider: Provider to use to get a tracer. If nil, global tracer provider is used.
//   - logger: Logger to use to publish debug logs. If nil, debug logs will be discarded.
//
// # Return
//
// A new, non-started heartbeat event source.
func NewHeartbeatEventSource(tracerProvider trace.TracerProvider, logger *log.Logger) *HeartbeatEventSource {
	if logger == nil {
		// Use a logger which does not print logs the provided one is nil
		logger = log.New(io.Discard, "", log.Flags())
	}
	if tracerProvider == nil {
		// Use global tracer provider if provided one is nil
		tracerProvider = otel.GetTracerProvider()
	}
	// Build and return the event source
	return &HeartbeatEventSource{
		publish: make(chan event.Event),
		stop:    nil,
		tracer:  tracerProvider.Tracer(tracing.PackageName+".sources.heartbeat", trace.WithInstrumentationVersion(tracing.PackageVersion)),
		logger:  logger,
	}
}

// # Description
//
// Provide the channel used by the event source to publish its events.
//
// # Implementation requirements & hints
//
//   - The method must return the same channel instance to all callers.
//   - The source must provide its publication channel even if it has not been started yet.
//   - The source must continue providing the same channel even if the source has been stopped.
//   - To achieve end to end event traceability, events are expected to embed tracing data as
//     described in the following documentation:
//     https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/extensions/distributed-tracing.md
//
// # Return
//
// The channel used by this source to publish its events.
func (source *HeartbeatEventSource) GetChannel() chan event.Event {
	return source.publish
}

// # Description
//
// Start the event source that will start publishing events on its publication channel.
//
// # Implementation requirements & hints
//
//   - The method must return an error if the source has already been started.
//   - The method must return an error if the provided context has been canceled or has expired.
//   - The method must return an error if the source has been stopped (no restart).
//   - The method must close the publication channel.
//
// # Inputs
//
//   - ctx: Context used for tracing purpose.
//
// # Return
//
// An error if the event source could not be started.
func (source *HeartbeatEventSource) Start(ctx context.Context) error {
	// Tracing: Start span
	ctx, span := source.tracer.Start(ctx, "start", trace.WithSpanKind(trace.SpanKindInternal))
	source.logger.Print("[SOURCE][HEARTBEAT]: starting event source")
	select {
	case <-ctx.Done():
		// Shortcut
		return tracing.HandleAndTraLogErr(span, source.logger, fmt.Errorf("[SOURCE][HEARTBEAT]: failed to start heartbeat event source: %w", ctx.Err()))
	default:
		// Create a separate parent context with a cancel function
		pctx := context.Background()
		pctx, source.stop = context.WithCancel(pctx)
		// Start the internal goroutine that will generate and publish events
		go publishHeartbeats(pctx, source.publish, source.tracer, source.logger)
		// Close span and exit
		span.SetStatus(codes.Ok, codes.Ok.String())
		span.End()
		return nil
	}
}

// # Description
//
// Stop the event source and close the publication channel.
//
// # Implementation requirements & hints
//
//   - The method must return an error if the source has not been started.
//   - The method must return an error if the source has already been stopped.
//   - The method must close the publication channel even if the underlying event source could
//     not properly closed.
//   - The closed publication channel must be kept: source must not be restarted (stale source).
//
// # Inputs
//
//   - ctx: Context used for tracing purpose.
//
// # Return
//
// An error if the event source could not be closed.
func (source *HeartbeatEventSource) Stop(ctx context.Context) error {
	// Tracing: Start span
	_, span := source.tracer.Start(ctx, "stop", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.SetStatus(codes.Ok, codes.Ok.String())
	defer span.End()
	// Call cancel function and exit
	source.stop()
	return nil
}

// # Description
//
// Publish heartbeats every 1 second on the publish channel until a cancellation signal is received
// through the provided parent context.
//
// # Inputs
//
//   - pctx: Parent context used to receive cancellation signals.
//   - publish: Channel on which heartbeats will be published
//   - tracer: Tracer used to instrument code
//   - logger: Logger used to publish debug logs
func publishHeartbeats(pctx context.Context, publish chan event.Event, tracer trace.Tracer, logger *log.Logger) {
	// Continuously publish heartbeat approx. every 1 second until cancellation signal is received.
	for {
		// Tracing: start span
		ctx, span := tracer.Start(context.Background(), "publish_heartbeat", trace.WithSpanKind(trace.SpanKindProducer))
		select {
		case <-pctx.Done():
			// Trace event
			span.AddEvent("exit")
			logger.Print("[SOURCE][HEARTBEAT]: cancellation signal received. Closing heartbeat source")
			// Close channel used to publish events. This will automatically unregister the event
			// source and cause all subscribers to unsubscribe.
			close(publish)
			// Close span and exit
			span.SetStatus(codes.Ok, codes.Ok.String())
			span.End()
			return
		default:
			// Publish a heartbeat event
			now := time.Now().UTC()
			logger.Printf("[SOURCE][HEARTBEAT]: publishing heartbeat %d", now.Unix())
			// Configure event
			heartbeat := Heartbeat{Timestamp: now.Unix()}
			event := event.New()
			event.SetData("application/json", heartbeat)
			event.SetID(strconv.FormatInt(now.Unix(), 10))
			event.SetType("heartbeat")
			event.SetSource("heartbeat")
			event.SetTime(now)
			otelObs.InjectDistributedTracingExtension(ctx, event)
			// Publish event on channel use a block write and watch for cancellation signals
			select {
			case publish <- event:
				logger.Printf("[SOURCE][HEARTBEAT]: published heartbeat %d", now.Unix())
				span.AddEvent("heartbeat_published", trace.WithAttributes(attribute.Int64("id", now.Unix())))
				span.SetStatus(codes.Ok, codes.Ok.String())
				span.End()
			case <-pctx.Done():
				// Trace and log error, close span and exit
				err := fmt.Errorf("[SOURCE][HEARTBEAT]: failed to publish heartbeat %d: cancellation signal has been received. Source will be closed", now.Unix())
				tracing.HandleAndTraLogErr(span, logger, err)
				// Close channel used to publish events. This will automatically unregister the event
				// source and cause all subscribers to unsubscribe.
				close(publish)
				// Exit
				return
			}
		}
		// Sleep 1 second before publishing next heartbeat
		time.Sleep(1 * time.Second)
	}
}
