package engine

import (
	"context"
	"sync"

	"github.com/juju/errors"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/internal/types"
	tele_api "github.com/temoto/vender/tele"
)

type task struct {
	fun  types.TaskFunc
	done chan error
	pri  tele_api.Priority
}

type Run struct {
	alive *alive.Alive
	q     chan task
	idle  sync.RWMutex
}

var _ types.Scheduler = &Run{} // compile-time interface test

func NewRunner() *Run {
	return &Run{
		alive: alive.NewAlive(),
		q:     make(chan task),
	}
}

func (r *Run) Loop(ctx context.Context, parent *alive.Alive) {
	defer r.alive.WaitTasks()
	myStop := r.alive.StopChan()
	parentStop := parent.StopChan()
	for parent.IsRunning() && r.alive.IsRunning() {
		select {
		case t := <-r.q:
			r.alive.Add(1)
			go r.doTask(ctx, t)

		case <-parentStop:
			r.alive.Stop()
			// return errors.Errorf("Run.Loop interrupted, ignore like EPIPE")
			return

		case <-myStop:
			// return errors.Errorf("Run.Loop interrupted, ignore like EPIPE")
			return
		}
	}
}

func (r *Run) Schedule(ctx context.Context, priority tele_api.Priority, fun types.TaskFunc) chan error {
	if !r.alive.IsRunning() {
		return nil
	}
	ch := make(chan error)
	r.q <- task{
		done: ch,
		fun:  fun,
		pri:  priority,
	}
	return ch
}

func (r *Run) ScheduleSync(ctx context.Context, priority tele_api.Priority, fun types.TaskFunc) error {
	if !r.alive.IsRunning() {
		return errors.Trace(types.ErrInterrupted)
	}
	r.alive.Add(1)
	return r.do(ctx, priority, fun)
}

func (r *Run) do(ctx context.Context, priority tele_api.Priority, fun types.TaskFunc) error {
	defer r.alive.Done()

	if wantIdle(priority) {
		r.idle.Lock()
		defer r.idle.Unlock()
	} else {
		r.idle.RLock()
		defer r.idle.RUnlock()
	}
	// May be stopped while waiting for lock
	select {
	case <-r.alive.StopChan():
		return errors.Trace(types.ErrInterrupted)
	default:
	}

	return fun(ctx)
}

func (r *Run) doTask(ctx context.Context, t task) {
	err := r.do(ctx, t.pri, t.fun)
	t.done <- err
	close(t.done)
}

func wantIdle(p tele_api.Priority) bool { return p&tele_api.Priority_IdleEngine != 0 }
