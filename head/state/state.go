// Package state let subsystems register lifecycle callbacks.
package state

import (
	"context"
	"log"
	"sync"

	"github.com/temoto/alive"
)

type Callback func(context.Context) error

var (
	glk        sync.Mutex
	onstart    []Callback
	onvalidate []Callback
	onstop     []Callback
)

func DoStart(ctx context.Context) []error    { return do(onstart, ctx, "start") }
func DoValidate(ctx context.Context) []error { return do(onvalidate, ctx, "validate") }
func DoStop(ctx context.Context) []error     { return do(onstop, ctx, "stop") }

func RegisterStart(fun Callback)    { register(&onstart, fun, "start") }
func RegisterValidate(fun Callback) { register(&onvalidate, fun, "validate") }
func RegisterStop(fun Callback)     { register(&onstop, fun, "stop") }

func Restart() {
	log.Println("restart requested")
	DoStop(context.Background())
}

func do(funs []Callback, ctx context.Context, tag string) []error {
	var errs []error = nil
	var lk sync.Mutex
	a := alive.NewAlive()
	a.Add(len(funs))
	safeAppend := func(fun Callback) {
		if err := fun(ctx); err != nil {
			lk.Lock()
			errs = append(errs, err)
			lk.Unlock()
		}
		a.Done()
	}
	for _, fun := range funs {
		go safeAppend(fun)
	}
	a.WaitTasks()
	return errs
}

func register(a *[]Callback, fun Callback, tag string) {
	glk.Lock()
	*a = append(*a, fun)
	glk.Unlock()
}
