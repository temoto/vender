package helpers

// Random synchronisation util stash

import (
	"sync"

	"github.com/temoto/alive/v2"
)

func AliveSub(root, leaf *alive.Alive) {
	select {
	case <-root.StopChan():
		leaf.Stop()
	case <-leaf.StopChan():
	}
}

func WithLock(l sync.Locker, f func()) {
	l.Lock()
	defer l.Unlock()
	f()
}

func WithLockError(l sync.Locker, f func() error) error {
	l.Lock()
	defer l.Unlock()
	return f()
}

type AtomicError struct {
	mu  sync.Mutex
	err error
	set bool
}

func (a *AtomicError) Load() (error, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err, a.set
}

// StoreOnce stores e only first time, returns same as Load() before modification.
func (a *AtomicError) StoreOnce(e error) (error, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	berr, bset := a.err, a.set
	if !bset {
		a.err, a.set = e, true
	}
	return berr, bset
}
