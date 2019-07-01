package vmc

import (
	"context"

	"github.com/coreos/go-systemd/daemon"
	"github.com/temoto/errors"
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
	g.Log.Debugf("config=%+v", g.Config())

	mdber, err := g.Mdber()
	if err != nil {
		err = errors.Annotate(err, "mdb init")
		return err
	}
	if err = mdber.BusResetDefault(); err != nil {
		err = errors.Annotate(err, "mdb bus reset")
		return err
	}

	moneysys := new(money.MoneySystem)
	if err = moneysys.Start(ctx); err != nil {
		err = errors.Annotate(err, "money system Start()")
		return err
	}

	// TODO func(dev Devicer) { dev.Init() && dev.Register() }
	// right now Enum does IO implicitly
	// FIXME hardware.Enum() but money system inits bill/coin devices explicitly
	evend.Enum(ctx, nil)

	menuMap := make(ui.Menu)
	if err = menuMap.Init(ctx); err != nil {
		err = errors.Annotate(err, "menuMap.Init")
		return err
	}
	g.Log.Debugf("uiFront len=%d", len(menuMap))

	uiFront := ui.NewUIFront(ctx, menuMap)

	moneysys.EventSubscribe(func(em money.Event) {
		g.Log.Debugf("money event: %s", em.String())
		uiFront.SetCredit(moneysys.Credit(ctx))

		switch em.Name() {
		case money.EventCredit:
		case money.EventAbort:
		default:
			panic("head: unknown money event: " + em.String())
		}
	})
	go vmc_common.TeleCommandLoop(ctx, moneysys)

	// FIXME
	g.Inventory.DisableAll()

	subcmd.SdNotify(daemon.SdNotifyReady)
	g.Log.Debugf("VMC init complete")

	subcmd.SdNotify("executing on_start")
	if err := g.Engine.ExecList(ctx, "on_start", g.Config().Engine.OnStart); err != nil {
		g.Log.Error(err)

		// TODO restart all hardware
		evend.Enum(ctx, nil)

		if err := g.Engine.ExecList(ctx, "on_start", g.Config().Engine.OnStart); err != nil {
			g.Log.Error(err)
			uiFront.SetBroken(true)
		}
	}

	vmc_common.UILoop(ctx, uiFront, moneysys, menuMap)
	return nil
}
