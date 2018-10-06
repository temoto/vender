package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/coreos/go-systemd/daemon"
	"github.com/temoto/alive"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/head/money"
	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/head/ui"
	"github.com/temoto/vender/msync"

	// invoke package init to register lifecycles
	_ "github.com/temoto/vender/head/kitchen"
	_ "github.com/temoto/vender/head/papa"
	_ "github.com/temoto/vender/head/telemetry"
)

// TODO decide
// seq := msync.NewSequence("head-init")
// seq.Append(msync.NewAction("", Hello))
// seq.Append(msync.MustGlobalAction("display-init"))
//
// seq.Start()
// time.Sleep(100 * time.Millisecond)
// seq.Abort()
//
// seq.Start()
// seq.Wait()
// time.Sleep(100 * time.Millisecond)

func foldErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	ss := make([]string, 0, len(errs))
	for _, e := range errs {
		if e != nil {
			ss = append(ss, e.Error())
		}
	}
	if len(ss) == 0 {
		return nil
	}
	return fmt.Errorf(strings.Join(ss, "\n"))
}

func main() {
	flagConfig := flag.String("config", "vender.hcl", "")
	flag.Parse()

	const logFlagsService = log.Lshortfile
	const logFlagsInteractive = log.Lshortfile | log.Ltime | log.Lmicroseconds
	if sdnotify("start") {
		// we're under systemd, assume systemd journal logging, remove timestamp
		log.SetFlags(logFlagsService)
	} else {
		log.SetFlags(logFlagsInteractive)
	}
	log.Println("hello")

	a := alive.NewAlive()
	a.Add(1)
	ctx := context.Background()
	ctx = context.WithValue(ctx, "alive", a)

	state.RegisterStop(func(ctx context.Context) error {
		a.Done()
		a.Stop()
		return nil
	})

	config := state.MustReadConfigFile(log.Fatal, *flagConfig)
	log.Printf("config=%+v", config)
	ctx = context.WithValue(ctx, "config", config)
	if err := foldErrors(state.DoValidate(ctx)); err != nil {
		log.Fatal(err)
	}

	mdber, err := mdb.NewMDB(config.Mdb.Uarter, config.Mdb.UartDevice, config.Mdb.UartBaudrate)
	if err != nil {
		log.Fatal(err)
	}
	mdber.SetDebug(config.Mdb.Debug)
	mdber.BreakCustom(200, 500)
	ctx = context.WithValue(ctx, "run/mdber", mdber)

	state.DoStart(ctx)
	sdnotify(daemon.SdNotifyReady)

	for a.IsRunning() {
		select {
		case <-a.StopChan():
		case em := <-money.Global.Events():
			log.Printf("money event: %s", em.String())
			switch em.Name() {
			case money.EventCredit:
				ui.Logf("money: credit %s", em.Amount().Format100I())
			case money.EventAbort:
				err := money.Global.Abort(context.Background())
				log.Printf("user requested abort err=%v", err)
			default:
				panic("head: unknown money event: " + em.String())
			}
		}
	}

	a.Wait()
}

func Hello(w *msync.MultiWait, args interface{}) (err error) {
	if w.IsDone() {
		log.Println("hello aborted")
		return
	}
	log.Println("hello begin")
	select {
	case <-time.After(1 * time.Second):
	case <-w.Chan():
	}
	if w.IsDone() {
		log.Println("hello aborted")
		return
	}
	log.Println("hello done")
	return
}

func sdnotify(s string) bool {
	ok, err := daemon.SdNotify(false, s)
	if err != nil {
		log.Fatal("sdnotify: ", err)
	}
	return ok
}
