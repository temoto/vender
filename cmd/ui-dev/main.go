// Temporary executable for developing UI
package main

import (
	"context"
	"flag"
	"os"

	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/log2"
)

var log = log2.NewStderr(log2.LDebug)

func main() {
	cmdline := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagConfig := cmdline.String("config", "vender.hcl", "")
	cmdline.Parse(os.Args[1:])

	log.SetFlags(log2.LInteractiveFlags)

	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	config := state.MustReadConfigFile(*flagConfig, log)
	log.Debugf("config=%+v", config)
	ctx = state.ContextWithConfig(ctx, config)

	log.Debugf("Init display")
	d := config.Global().Hardware.HD44780.Display
	d.SetLine1("loaded")

	moneysys := new(money.MoneySystem)
	moneysys.Start(ctx)

	for {
		menu := ui.NewUIMenu(ctx)
		menu.SetCredit(moneysys.Credit(ctx))

		stopCh := menu.StopChan()
		go func() {
			for {
				select {
				case <-stopCh:
					return
				case em := <-moneysys.Events():
					log.Debugf("money event: %s", em.String())
					switch em.Name() {
					case money.EventCredit:
						menu.SetCredit(moneysys.Credit(ctx))
					case money.EventAbort:
						err := moneysys.Abort(ctx)
						log.Infof("user requested abort err=%v", err)
						menu.SetCredit(moneysys.Credit(ctx))
					default:
						panic("head: unknown money event: " + em.String())
					}
				}
			}
		}()

		result := menu.Run()
		log.Debugf("result=%#v", result)
	}
}
