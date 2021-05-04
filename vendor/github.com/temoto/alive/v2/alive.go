// alive helps servers to coordinate graceful or fast stopping
package alive

import (
	"fmt"
	"sync"
	"sync/atomic"
)

const (
	stateRunning = iota
	stateStopping
	stateFinished
)

const NotRunning = "Alive.Add(): need state Running. Attempted to run new task after Stop()"

func stateString(s uint32) string {
	switch s {
	case stateRunning:
		return "running"
	case stateStopping:
		return "stopping"
	case stateFinished:
		return "finished"
	}
	return "unknown!"
}

func formatBugState(state uint32, source string) string {
	return fmt.Sprintf(`Bug in package 'alive': unexpected state value %d (%s) in %s.
Please post minimal reproduction code on Github issues, see package import path.`,
		state, stateString(state), source)
}

// Alive waits for subtasks, coordinate graceful or fast shutdown.
// Exported for easy/fast direct references in your code.
// Zero `Alive{}` is not usable ever. You *must* call `NewAlive()`.
type Alive struct {
	wg       sync.WaitGroup
	state    uint32
	lk       sync.Mutex
	chStop   chan struct{}
	chFinish chan struct{}
}

func NewAlive() *Alive {
	self := &Alive{
		state:    stateRunning,
		chStop:   make(chan struct{}),
		chFinish: make(chan struct{}),
	}
	return self
}

// Corresponds to `sync.WaitGroup.Add()`
// Returns `true` on successful increment or `false` after `Stop()`.
func (self *Alive) Add(delta int) bool {
	// pessimistic fast path
	if !self.IsRunning() {
		return false
	}

	self.lk.Lock()
	defer self.lk.Unlock()
	if !self.IsRunning() {
		return false
	}
	self.wg.Add(delta)
	return true
}

// Corresponds to `sync.WaitGroup.Done()`
func (self *Alive) Done() {
	state := atomic.LoadUint32(&self.state)
	switch state {
	case stateRunning, stateStopping:
		self.wg.Done()
		return
	}
	panic(formatBugState(state, "Done"))
}

func (self *Alive) IsRunning() bool  { return atomic.LoadUint32(&self.state) == stateRunning }
func (self *Alive) IsStopping() bool { return atomic.LoadUint32(&self.state) == stateStopping }
func (self *Alive) IsFinished() bool { return atomic.LoadUint32(&self.state) == stateFinished }

// Stop puts Alive into Stopping mode, closes `StopChan()` and returns immediately.
// After all pending tasks `Done()` state changes to Finished and unblocks all `Wait*` calls/channel.
// Multiple and concurrent calls are allowed and produce same result.
func (self *Alive) Stop() {
	self.lk.Lock()
	defer self.lk.Unlock()
	state := atomic.LoadUint32(&self.state)
	switch state {
	case stateRunning:
		atomic.StoreUint32(&self.state, stateStopping)
		close(self.chStop)
		go self.finish()
		return
	case stateStopping, stateFinished:
		return
	}
	panic(formatBugState(state, "Stop"))
}

func (self *Alive) finish() {
	self.WaitTasks()
	atomic.StoreUint32(&self.state, stateFinished)
	close(self.chFinish)
}

// StopChan is closed when `Stop()` is called.
func (self *Alive) StopChan() <-chan struct{} {
	return self.chStop
}

// WaitChan is closed when both `Stop()` is called **and** all pending tasks done.
// Multiple and concurrent `Wait()`/`<-WaitChan()` are allowed and produce same result.
func (self *Alive) WaitChan() <-chan struct{} {
	return self.chFinish
}

// Corresponds to `sync.WaitGroup.Wait()`
// Multiple and concurrent calls allowed and produce same result.
func (self *Alive) WaitTasks() {
	self.wg.Wait()
}

// Wait returns after both `Stop()` is called **and** all pending tasks done.
// Multiple and concurrent `Wait()`/`<-WaitChan()` are allowed and produce same result.
func (self *Alive) Wait() {
	<-self.chFinish
}

func (self *Alive) String() string {
	return fmt.Sprintf("state=%s",
		stateString(atomic.LoadUint32(&self.state)),
	)
}
