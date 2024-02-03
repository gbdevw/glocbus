package main

import (
	"log"

	"github.com/gbdevw/glocbus/example/providers"
	"go.uber.org/fx"
)

func main() {
	fx.New(
		fx.Provide(log.Default),
		fx.Provide(providers.ProvideTraceProvider),
		fx.Provide(new(providers.GlocbusSingletonProvider).ProvideGlocbusInstance),
		fx.Invoke(providers.ProvideEventSource),          // Force sources to be started
		fx.Invoke(providers.ProvideHeartbeatSubscribers), // Force subscribers to be started after sources
	).Run()
}
