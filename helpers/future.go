// Based on https://github.com/256dpi/gomqtt/blob/e7823dfd0958f968b8e69eb1bf235456316c54fb/client/future/future.go
// with completed/cancelled channels exported
// which allows to wait on result in custom select statement.

package helpers

import (
	"sync"
)

type Future struct {
	result    interface{}
	completed chan struct{}
	cancelled chan struct{}
	done      bool
	mutex     sync.Mutex
}

func NewFuture() *Future {
	return &Future{
		completed: make(chan struct{}),
		cancelled: make(chan struct{}),
	}
}

func (f *Future) Cancelled() <-chan struct{} { return f.cancelled }
func (f *Future) Completed() <-chan struct{} { return f.completed }

func (f *Future) Complete(result interface{}) bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.done {
		return false
	}

	f.result = result
	close(f.completed)
	f.done = true
	return true
}

func (f *Future) Cancel(result interface{}) bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.done {
		return false
	}

	f.result = result
	close(f.cancelled)
	f.done = true
	return true
}

func (f *Future) Result() interface{} {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.result
}
