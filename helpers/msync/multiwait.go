package msync

import "sync"

// Internal structure, may be changed.
// Requirements for this data structure:
// * WaitTouch() blocks until next Touch() or Done()
// * WaitDone() blocks until Done()
// * Touch() wakes up everybody
// * Done() wakes up only those who waited for it
// * Reset() allows to wait for Done() again
type MultiWait struct {
	done bool
	err  error
	cond sync.Cond
}

func NewMultiWait() *MultiWait {
	return &MultiWait{
		cond: sync.Cond{L: new(sync.Mutex)},
	}
}

func (self *MultiWait) WaitTouch() {
	self.cond.L.Lock()
	self.cond.Wait()
	self.cond.L.Unlock()
}

func (self *MultiWait) Touch() {
	self.cond.Broadcast()
}

func (self *MultiWait) WaitDone() error {
	self.cond.L.Lock()
	defer self.cond.L.Unlock()
	for {
		if self.done {
			return self.err
		}
		self.cond.Wait()
	}
}

func (self *MultiWait) Done(err error) {
	self.cond.L.Lock()
	if !self.done {
		self.done = true
		self.err = err
	}
	self.cond.L.Unlock()
	self.cond.Broadcast()
}

func (self *MultiWait) Reset() {
	self.cond.L.Lock()
	self.done = false
	self.err = nil
	self.cond.L.Unlock()
}

func (self *MultiWait) Chan() chan error {
	ch := make(chan error)
	go func() {
		ch <- self.WaitDone()
	}()
	return ch
}

func (self *MultiWait) IsDone() (out bool) {
	self.cond.L.Lock()
	out = self.done
	self.cond.L.Unlock()
	return
}
