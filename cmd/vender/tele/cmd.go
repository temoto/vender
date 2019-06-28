package tele

import (
	"context"
	"fmt"
	"time"

	"github.com/temoto/vender/cmd/vender/subcmd"
	"github.com/temoto/vender/head/tele"
	"github.com/temoto/vender/state"
)

var Mod = subcmd.Mod{Name: "tele", Main: Main}

func Main(ctx context.Context, config *state.Config) error {
	g := state.GetGlobal(ctx)
	synthConfig := &state.Config{
		Tele: config.Tele,
	}
	g.MustInit(ctx, synthConfig)

	telesys := &state.GetGlobal(ctx).Tele
	go func() {
		stopCh := g.Alive.StopChan()
		for {
			select {
			case <-stopCh:
				return
			case cmd := <-telesys.CommandChan():
				switch cmd.Task.(type) {
				case *tele.Command_Abort:
					g.Log.Infof("tele command abort")
					telesys.CommandReplyErr(&cmd, nil)
					g.Log.Infof("tele command abort reply sent")
				case *tele.Command_SetGiftCredit:
					g.Log.Infof("tele command setgiftcredit")
				}
			}
		}
	}()

	g.Inventory.DisableAll()

	g.Log.Debugf("tele init complete, running")
	for g.Alive.IsRunning() {
		g.Log.Debugf("before telesys")
		telesys.Error(fmt.Errorf("tele tick"))
		time.Sleep(5 * time.Second)
		// time.Sleep(99 * time.Millisecond)
	}

	g.Alive.Wait()
	return nil
}
