package providers

import (
	"github.com/gbdevw/glocbus"
	"go.opentelemetry.io/otel/trace"
)

// Type for a provider that manages a glocbus instance as a singleton
type GlocbusSingletonProvider struct {
	bus *glocbus.Glocbus
}

// Provide the managed Glocbus instance
func (provider *GlocbusSingletonProvider) ProvideGlocbusInstance(tracerProvider trace.TracerProvider) glocbus.EventBusInterface {
	if provider.bus == nil {
		provider.bus = glocbus.NewGlocbus(tracerProvider)
	}
	return provider.bus
}
