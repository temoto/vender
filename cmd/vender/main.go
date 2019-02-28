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
	iodin "github.com/temoto/iodin/client/go-iodin"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/head/kitchen"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/papa"
	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/head/telemetry"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

type systems struct {
	kitchen   kitchen.KitchenSystem
	money     money.MoneySystem
	papa      papa.PapaSystem
	telemetry telemetry.TelemetrySystem
	ui        ui.UISystem
}

var log = log2.NewStderr(log2.LDebug)

func main() {
	cmdline := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagConfig := cmdline.String("config", "vender.hcl", "")
	flagUarter := cmdline.String("uarter", "file", "")
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

	lifecycle.RegisterSystem(&sys.kitchen)
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

	if *flagUarter == "iodin" {
		iodin, err := iodin.NewClient(config.Hardware.IodinPath)
		if err != nil {
			err = errors.Annotatef(err, "config: mdb.uart_driver=%s iodin_path=%s", config.Hardware.Mdb.UartDriver, config.Hardware.IodinPath)
			log.Fatal(err)
		}
		config.Hardware.Mdb.Uarter = mdb.NewIodinUart(iodin)
		config.Hardware.Mdb.UartDevice = "\x0f\x0e"
	}

	mdber, err := mdb.NewMDB(config.Hardware.Mdb.Uarter, config.Hardware.Mdb.UartDevice, log.Clone(log2.LError))
	if err != nil {
		log.Fatal(errors.ErrorStack(err))
	}
	if config.Hardware.Mdb.Log {
		mdber.Log.SetLevel(log2.LDebug)
	}
	mdber.BreakCustom(200*time.Millisecond, 500*time.Millisecond)
	ctx = context.WithValue(ctx, mdb.ContextKey, mdber)

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
				sys.ui.Logf("money: credit %s", em.Amount().Format100I())
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
