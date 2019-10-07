package ui

import (
	"sync/atomic"
	"time"
)

const lockPoll = 300 * time.Millisecond

func (self *UI) Locked() bool {
	return atomic.LoadInt32(&self.locked) > 0
}

func (self *UI) LockWait() bool {
	self.g.Log.Debugf("LockWait")
	atomic.AddInt32(&self.locked, 1)
	select {
	case self.lockedch <- struct{}{}:
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
	new := atomic.AddInt32(&self.locked, -1)
	if new < 0 {
		// Support concurrent LockEnd
		atomic.StoreInt32(&self.locked, 0)
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
	atomic.StoreInt32(&self.locked, 0)
	for self.g.Alive.IsRunning() && (self.State() == StateLocked) {
		time.Sleep(lockPoll)
	}
}

func (self *UI) LockDuration(d time.Duration) bool {
	if !self.LockWait() {
		return false
	}
	time.Sleep(d)
	self.LockDecrement()
	return true
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
