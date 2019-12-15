package ui

// UI lock allows dynamic override of UI state workflow.
// Use cases:
// - graceful shutdown waits until user interaction is complete
// - remote management shows users machine is not available and prevents interaction

import (
	"sync/atomic"
	"time"
)

const lockPoll = 300 * time.Millisecond

type uiLock struct {
	ch   chan struct{}
	sem  int32
	next State
}

func (l *uiLock) locked() bool {
	return atomic.LoadInt32(&l.sem) > 0
}

func (self *UI) LockFunc(fun func()) bool {
	if !self.LockWait() {
		return false
	}
	defer self.LockDecrement()
	fun()
	return true
}

func (self *UI) LockWait() bool {
	self.g.Log.Debugf("LockWait")
	atomic.AddInt32(&self.lock.sem, 1)
	select {
	case self.lock.ch <- struct{}{}:
	default:
	}
	for self.g.Alive.IsRunning() {
		time.Sleep(lockPoll)
		if self.State() == StateLocked {
			return true
		}
	}
	return false
}

func (self *UI) LockDecrement() {
	self.g.Log.Debugf("LockDecrement")
	new := atomic.AddInt32(&self.lock.sem, -1)
	if new < 0 {
		// Support concurrent LockEnd
		atomic.StoreInt32(&self.lock.sem, 0)
		new = 0
	}
	if new == 0 {
		for self.g.Alive.IsRunning() && (self.State() == StateLocked) {
			time.Sleep(lockPoll)
		}
	}
}

// Stop locked state ignoring call balance
func (self *UI) LockEnd() {
	self.g.Log.Debugf("LockEnd")
	atomic.StoreInt32(&self.lock.sem, 0)
	for self.g.Alive.IsRunning() && (self.State() == StateLocked) {
		time.Sleep(lockPoll)
	}
}

// Avoid interrupting some important states
// TODO configurable
func (self *UI) checkLockPriority(s State) bool {
	if s == StateFrontAccept {
		return false
	}
	if s >= StateServiceBegin && s <= StateServiceEnd {
		return false
	}
	return true
}
