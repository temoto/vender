package vmc

import (
	"context"

	"github.com/coreos/go-systemd/daemon"
	"github.com/juju/errors"
	"github.com/temoto/vender/cmd/vender/subcmd"
	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/head/vmc_common"
	"github.com/temoto/vender/state"
)

var Mod = subcmd.Mod{Name: "vmc", Main: Main}

func Main(ctx context.Context, config *state.Config) error {
	g := state.GetGlobal(ctx)
	g.MustInit(ctx, config)
	g.Log.Debugf("config=%+v", g.Config)

	mdber, err := g.Mdber()
	if err != nil {
		err = errors.Annotate(err, "mdb init")
		return err
	}
	if err = mdber.BusResetDefault(); err != nil {
		err = errors.Annotate(err, "mdb bus reset")
		return err
	}

	display := g.MustDisplay()
	display.SetLines("boot", g.Config.UI.Front.MsgWait)

	moneysys := new(money.MoneySystem)
	if err = moneysys.Start(ctx); err != nil {
		err = errors.Annotate(err, "money system Start()")
		return err
	}

	// TODO func(dev Devicer) { dev.Init() && dev.Register() }
	// right now Enum does IO implicitly
	// FIXME hardware.Enum() but money system inits bill/coin devices explicitly
	evend.Enum(ctx, nil)

	ui := ui.UI{State: ui.StateBoot}
	if err := ui.Init(ctx); err != nil {
		err = errors.Annotate(err, "ui Init()")
		return err
	}

	go vmc_common.TeleCommandLoop(ctx)

	subcmd.SdNotify(daemon.SdNotifyReady)
	g.Log.Debugf("VMC init complete")

	ui.Loop(ctx)
	return nil
}
