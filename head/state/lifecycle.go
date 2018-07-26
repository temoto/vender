// Let subsystems register lifecycle callbacks.
package state

import (
	"context"
	"os"
	"sync"
	"syscall"
)

type Callback func(context.Context) error

var (
	glk      sync.Mutex
	onstart  []Callback
	onreload []Callback
	onstop   []Callback
)

func register(a *[]Callback, fun Callback) {
	glk.Lock()
	onstart = append(onstart, fun)
	glk.Unlock()
}
func RegisterStart(fun Callback)  { register(&onstart, fun) }
func RegisterReload(fun Callback) { register(&onreload, fun) }
func RegisterStop(fun Callback)   { register(&onstop, fun) }

func do(a []Callback, ctx context.Context) []error {
	var errs []error = nil
	var lk sync.Mutex
	safeAppend := func(fun Callback) {
		if err := fun(ctx); err != nil {
			lk.Lock()
			errs = append(errs, err)
			lk.Unlock()
		}
	}
	for _, fun := range a {
		go safeAppend(fun)
	}
	return errs
}
func DoStart(ctx context.Context) []error  { return do(onstart, ctx) }
func DoReload(ctx context.Context) []error { return do(onreload, ctx) }
func DoStop(ctx context.Context) []error   { return do(onstop, ctx) }

func Restart() {
	DoStop(context.Background())
	syscall.Kill(os.Getpid(), syscall.SIGQUIT)
}
