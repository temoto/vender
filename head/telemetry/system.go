package telemetry

import (
	"context"
)

type TelemetrySystem struct {
	// TODO evaluate statsd, mqtt
}

func (self *TelemetrySystem) String() string                     { return "telemetry" }
func (self *TelemetrySystem) Validate(ctx context.Context) error { return nil }
func (self *TelemetrySystem) Start(ctx context.Context) error    { return nil }
func (self *TelemetrySystem) Stop(ctx context.Context) error     { return nil }
