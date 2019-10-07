package broken

import (
	"context"

	"github.com/coreos/go-systemd/daemon"
	"github.com/juju/errors"
	"github.com/temoto/vender/cmd/vender/subcmd"
	"github.com/temoto/vender/head/money"
	tele_api "github.com/temoto/vender/head/tele/api"
	"github.com/temoto/vender/state"
)

var Mod = subcmd.Mod{Name: "broken", Main: Main}

func Main(ctx context.Context, config *state.Config) error {
	g := state.GetGlobal(ctx)
	g.MustInit(ctx, config)

	display := g.MustDisplay()
	display.SetLines("boot", g.Config.UI.Front.MsgWait)

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
