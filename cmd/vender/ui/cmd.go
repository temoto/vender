// Helper for developing vender user interfaces
package ui

import (
	"context"
	"time"

	"github.com/temoto/errors"
	"github.com/temoto/vender/cmd/vender/subcmd"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/head/vmc_common"
	"github.com/temoto/vender/state"
)

var Mod = subcmd.Mod{Name: "ui", Main: Main}

func Main(ctx context.Context, config *state.Config) error {
	g := state.GetGlobal(ctx)
	g.MustInit(ctx, config)
	g.Log.Debugf("config=%+v", g.Config())

	g.Log.Debugf("Init display")
	display := g.Hardware.HD44780.Display

	// helper to display all CLCD characters
	var bb [32]byte
	for b0 := 0; b0 < 256/len(bb); b0++ {
		for i := 0; i < len(bb); i++ {
			bb[i] = byte(b0*len(bb) + i)
		}
		display.SetLinesBytes(bb[:16], bb[16:])
		time.Sleep(3 * time.Second)
	}

	moneysys := new(money.MoneySystem)
	if err := moneysys.Start(ctx); err != nil {
		err = errors.Annotate(err, "money system Start()")
		return err
	}

	menuMap := make(ui.Menu)
	menuMap.Add(1, "chai", g.Config().ScaleU(3),
		engine.Func0{F: func() error {
			display.SetLines("спасибо", "готовим...")
			time.Sleep(7 * time.Second)
			display.SetLines("успех", "спасибо")
			time.Sleep(3 * time.Second)
			return nil
		}})
	menuMap.Add(2, "coffee", g.Config().ScaleU(5),
		engine.Func0{F: func() error {
			display.SetLines("спасибо", "готовим...")
			time.Sleep(7 * time.Second)
			display.SetLines("успех", "спасибо")
			time.Sleep(3 * time.Second)
			return nil
		}})

	uiFront := ui.NewUIFront(ctx, menuMap)

	moneysys.EventSubscribe(func(em money.Event) {
		uiFront.SetCredit(moneysys.Credit(ctx))
		g.Log.Debugf("money event: %s", em.String())
		moneysys.AcceptCredit(ctx, menuMap.MaxPrice())
	})

	go vmc_common.TeleCommandLoop(ctx, moneysys)

	g.Log.Debugf("init complete, enter main loop")
	vmc_common.UILoop(ctx, uiFront, moneysys, menuMap)
	return nil
}
