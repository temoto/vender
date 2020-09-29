package tele

import (
	"context"

	"github.com/temoto/vender/log2"
	tele_config "github.com/temoto/vender/tele/config"
)

type Noop struct{}

var _ Teler = Noop{} // compile-time interface test

func (Noop) Init(context.Context, *log2.Log, tele_config.Config) error { return nil }

func (Noop) Close() {}

func (Noop) Error(error) {}

func (Noop) State(State) {}

func (Noop) StatModify(func(*Stat)) {}

func (Noop) Report(ctx context.Context, serviceTag bool) error { return nil }

func (Noop) Transaction(*Telemetry_Transaction) {}
