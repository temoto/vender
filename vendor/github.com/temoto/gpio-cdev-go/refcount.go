package gpio

import (
	"sync/atomic"
	"syscall"
)

// Atomic reference counting fd closer
// If Linux is okay with random chip/handle/event fd closing order,
// this mechanism is redundant and should be removed. TODO verify
type fdArc struct {
	fd int
	c  int32
	e  chan error
}

func newFdArc(fd int) fdArc {
	return fdArc{fd: fd, c: 1, e: make(chan error, 1)}
}

func (f *fdArc) incref() bool {
	nc := atomic.AddInt32(&f.c, 1)
	return nc > 1
}

func (f *fdArc) decref() {
	nc := atomic.AddInt32(&f.c, -1)
	if nc == 0 {
		err := syscall.Close(f.fd)
		select {
		case f.e <- err:
		default:
			panic("code error err chan is busy")
		}
	} else if nc < 0 {
		panic("code error excess fdArc.decref")
	}
}

func (f *fdArc) wait() error {
	err := <-f.e
	close(f.e)
	return err
}
