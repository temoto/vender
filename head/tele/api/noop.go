package tele_api

import (
	"context"

	tele_config "github.com/temoto/vender/head/tele/config"
	"github.com/temoto/vender/log2"
)

type Noop struct{}

var _ Teler = Noop{}

func (Noop) Init(context.Context, *log2.Log, tele_config.Config) error { return nil }

func (Noop) Error(error) {}

func (Noop) State(State) {}

func (Noop) StatModify(func(*Stat)) {}

func (Noop) Transaction(Telemetry_Transaction) {}
