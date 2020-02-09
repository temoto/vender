// Main, user facing mode of operation.
package vmc

import (
	"context"

	"github.com/coreos/go-systemd/daemon"
	"github.com/juju/errors"
	"github.com/temoto/vender/cmd/vender/subcmd"
	"github.com/temoto/vender/hardware"
	"github.com/temoto/vender/internal/money"
	"github.com/temoto/vender/internal/state"
	"github.com/temoto/vender/internal/ui"
	tele_api "github.com/temoto/vender/tele"
)

var VmcMod = subcmd.Mod{Name: "vmc", Main: VmcMain}
var BrokenMod = subcmd.Mod{Name: "broken", Main: BrokenMain}

func VmcMain(ctx context.Context, config *state.Config) error {
	g := state.GetGlobal(ctx)
	g.MustInit(ctx, config)

	display := g.MustDisplay()
	display.SetLines("boot "+g.BuildVersion, g.Config.UI.Front.MsgWait)

	mdbus, err := g.Mdb()
	if err != nil {
		return errors.Annotate(err, "mdb init")
	}
	if err = mdbus.ResetDefault(); err != nil {
		return errors.Annotate(err, "mdb bus reset")
	}

	if err = hardware.Enum(ctx); err != nil {
		return errors.Annotate(err, "hardware enum")
	}

	moneysys := new(money.MoneySystem)
	if err := moneysys.Start(ctx); err != nil {
		return errors.Annotate(err, "money system Start()")
	}

	ui := ui.UI{}
	if err := ui.Init(ctx); err != nil {
		return errors.Annotate(err, "ui Init()")
	}

	subcmd.SdNotify(daemon.SdNotifyReady)
	g.Log.Debugf("VMC init complete")

	ui.Loop(ctx)
	return nil
}

func BrokenMain(ctx context.Context, config *state.Config) error {
	g := state.GetGlobal(ctx)
	g.MustInit(ctx, config)

	display := g.MustDisplay()
	display.SetLines("boot "+g.BuildVersion, g.Config.UI.Front.MsgWait)

	subcmd.SdNotify(daemon.SdNotifyReady)

	if mdbus, err := g.Mdb(); err != nil || mdbus == nil {
		if err == nil {
			err = errors.Errorf("hardware problem, see logs")
		}
		err = errors.Annotate(err, "mdb init")
		g.Error(err)
	} else {
		if err = mdbus.ResetDefault(); err != nil {
			err = errors.Annotate(err, "mdb bus reset")
			g.Error(err)
		}
		if err = hardware.Enum(ctx); err != nil {
			err = errors.Annotate(err, "hardware enum")
			g.Error(err)
		}
		moneysys := new(money.MoneySystem)
		if err := moneysys.Start(ctx); err != nil {
			err = errors.Annotate(err, "money system Start()")
			g.Error(err)
		} else {
			g.Error(moneysys.Abort(ctx))
		}
	}

	g.Tele.State(tele_api.State_Problem)
	display.SetLines(g.Config.UI.Front.MsgStateBroken, "")
	g.Error(errors.Errorf("critical daemon broken mode"))
	g.Alive.Wait()
	return nil
}
