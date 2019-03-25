package main

import (
	"context"
	"flag"
	"os"
	"runtime"
	"time"

	"github.com/coreos/go-systemd/daemon"
	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/papa"
	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/head/telemetry"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

type systems struct {
	money     money.MoneySystem
	papa      papa.PapaSystem
	telemetry telemetry.TelemetrySystem
	ui        ui.UISystem
}

var log = log2.NewStderr(log2.LDebug)

func main() {
	cmdline := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagConfig := cmdline.String("config", "vender.hcl", "")
	cmdline.Parse(os.Args[1:])

	logFlags := log2.LInteractiveFlags
	if sdnotify("start") {
		// under systemd assume systemd journal logging, no timestamp
		logFlags = log2.LServiceFlags
	}
	log.SetFlags(logFlags)
	log.Debugf("hello")

	a := alive.NewAlive()
	lifecycle := state.NewLifecycle(log) // validate/start/stop events
	sys := systems{}

	lifecycle.RegisterSystem(&sys.money)
	lifecycle.RegisterSystem(&sys.papa)
	lifecycle.RegisterSystem(&sys.telemetry)
	lifecycle.RegisterSystem(&sys.ui)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "alive", a)
	ctx = context.WithValue(ctx, "lifecycle", lifecycle)
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	ctx = context.WithValue(ctx, engine.ContextKey, engine.NewEngine(ctx))

	config := state.MustReadConfigFile(*flagConfig, log)
	log.Debugf("config=%+v", config)
	ctx = state.ContextWithConfig(ctx, config)
	if err := helpers.FoldErrors(lifecycle.OnValidate.Do(ctx)); err != nil {
		log.Fatal(errors.ErrorStack(err))
	}

	config.Global().Hardware.Mdb.Mdber.BreakCustom(200*time.Millisecond, 500*time.Millisecond)

	// TODO func(dev Devicer) { dev.Init() && dev.Register() }
	// right now Enum does IO implicitly
	evend.Enum(ctx, nil)

	if err := helpers.FoldErrors(lifecycle.OnStart.Do(ctx)); err != nil {
		log.Fatal(err)
	}
	sdnotify(daemon.SdNotifyReady)

	log.Debugf("systems init complete, running")
	stopCh := a.StopChan()
	for a.IsRunning() {
		select {
		case <-stopCh:
		case em := <-sys.money.Events():
			log.Debugf("money event: %s", em.String())
			switch em.Name() {
			case money.EventCredit:
				sys.ui.Logf("credit:%s", em.Amount().Format100I())
			case money.EventAbort:
				err := sys.money.Abort(ctx)
				log.Infof("user requested abort err=%v", err)
			default:
				panic("head: unknown money event: " + em.String())
			}
		}
	}

	a.Wait()
	// maybe these KeepAlive() are redundant
	runtime.KeepAlive(a)
	runtime.KeepAlive(ctx)
	runtime.KeepAlive(lifecycle)
	runtime.KeepAlive(&sys)
}

func sdnotify(s string) bool {
	ok, err := daemon.SdNotify(false, s)
	if err != nil {
		log.Fatal("sdnotify: ", errors.ErrorStack(err))
	}
	return ok
}
