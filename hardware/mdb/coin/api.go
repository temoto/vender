package coin

import (
	"context"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/state"
)

const deviceName = "mdb.coin"

func Enum(ctx context.Context) error {
	g := state.GetGlobal(ctx)
	dev := &CoinAcceptor{}
	// TODO dev.init() without IO
	// TODO g.RegisterDevice(deviceName, dev, dev.Probe)
	return g.RegisterDevice(deviceName, dev, func() error { return dev.init(ctx) })
}

type Coiner interface {
	AcceptMax(currency.Amount) engine.Doer
	Run(context.Context, *alive.Alive, func(money.PollItem) bool)
	ExpansionDiagStatus(*DiagResult) error
	SupportedNominals() []currency.Nominal
	NewGive(currency.Amount, bool, *currency.NominalGroup) engine.Doer
	TubeStatus() error
	Tubes() *currency.NominalGroup
}

var _ Coiner = &CoinAcceptor{}
var _ Coiner = Stub{}

type Stub struct{}

func (Stub) AcceptMax(currency.Amount) engine.Doer {
	return engine.Fail{E: errors.NotSupportedf("coin.Stub.AcceptMax")}
}

func (Stub) Run(ctx context.Context, alive *alive.Alive, fun func(money.PollItem) bool) {
	fun(money.PollItem{
		Status: money.StatusFatal,
		Error:  errors.NotSupportedf("coin.Stub.Run"),
	})
	if alive != nil {
		alive.Done()
	}
}

func (Stub) ExpansionDiagStatus(*DiagResult) error {
	return errors.NotSupportedf("coin.Stub.ExpansionDiagStatus")
}

func (Stub) SupportedNominals() []currency.Nominal { return nil }

func (Stub) NewGive(currency.Amount, bool, *currency.NominalGroup) engine.Doer {
	// return engine.Fail{E: errors.NotSupportedf("coin.Stub.NewGive")}
	return engine.Nothing{}
}

// func (Stub) TubeStatus() error { return errors.NotSupportedf("coin.Stub.TubeStatus") }
func (Stub) TubeStatus() error { return nil }

func (Stub) Tubes() *currency.NominalGroup {
	ng := &currency.NominalGroup{}
	ng.SetValid(nil)
	return ng
}
