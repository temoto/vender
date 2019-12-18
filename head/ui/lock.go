package ui

// UI lock allows dynamic override of UI state workflow.
// Use cases:
// - graceful shutdown waits until user interaction is complete
// - remote management shows users machine is not available and prevents interaction

import (
	"sync/atomic"
	"time"

	tele_api "github.com/temoto/vender/head/tele/api"
)

const lockPoll = 300 * time.Millisecond

type uiLock struct {
	ch   chan struct{}
	pri  uint32
	sem  int32
	next State
}

func (self *UI) LockFunc(pri tele_api.Priority, fun func()) bool {
	if !self.LockWait(pri) {
		return false
	}
	defer self.LockDecrementWait()
	fun()
	return true
}

func (self *UI) LockWait(pri tele_api.Priority) bool {
	self.g.Log.Debugf("LockWait")
	newSem := atomic.AddInt32(&self.lock.sem, 1)
	oldPri := self.lock.priority()
	if newSem == 1 || (newSem > 1 && pri != oldPri && pri == tele_api.Priority_Now) {
		atomic.StoreUint32(&self.lock.pri, uint32(pri))
	}
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

func (self *UI) LockDecrementWait() {
	self.g.Log.Debugf("LockDecrementWait")
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

func (self *UI) checkInterrupt(s State) bool {
	if !self.lock.locked() {
		return false
	}

	interrupt := true
	if self.lock.priority()&tele_api.Priority_IdleUser != 0 {
		interrupt = !(s > StateFrontBegin && s < StateFrontEnd) &&
			!(s >= StateServiceBegin && s <= StateServiceEnd)
	}
	return interrupt
}

func (l *uiLock) locked() bool                { return atomic.LoadInt32(&l.sem) > 0 }
func (l *uiLock) priority() tele_api.Priority { return tele_api.Priority(atomic.LoadUint32(&l.pri)) }
