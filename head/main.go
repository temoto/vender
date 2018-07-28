package main

import (
	"context"
	"log"
	"time"

	"github.com/coreos/go-systemd/daemon"
	"github.com/temoto/senderbender/alive"
	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/msync"

	// invoke package init to register lifecycles
	_ "github.com/temoto/vender/head/kitchen"
	_ "github.com/temoto/vender/head/money"
	_ "github.com/temoto/vender/head/papa"
	_ "github.com/temoto/vender/head/telemetry"
	_ "github.com/temoto/vender/head/ui"
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

func main() {
	logflags := log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile | log.Lshortfile
	if sdnotify("start") {
		// we're under systemd, assume systemd journal logging, remove timestamp
		logflags ^= log.Ldate | log.Ltime
	}
	log.SetFlags(logflags)
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

	config, err := state.ReadConfigFile("f")
	if err != nil {
		log.Fatal(err)
	}
	ctx = context.WithValue(ctx, "config", config)
	state.DoValidate(ctx)
	state.DoStart(ctx)
	sdnotify(daemon.SdNotifyReady)

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
