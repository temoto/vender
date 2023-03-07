package gpio

import (
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/juju/errors"
)

func (c *chip) GetLineEvent(line uint32, flag RequestFlag, events EventFlag, consumerLabel string) (Eventer, error) {
	if !c.fa.incref() {
		return nil, ErrClosed
	}

	req := EventRequest{
		LineOffset:   line,
		RequestFlags: GPIOHANDLE_REQUEST_INPUT | flag,
		EventFlags:   events,
	}
	copy(req.ConsumerLabel[:], []byte(consumerLabel))

	err := RawGetLineEvent(c.fa.fd, &req)
	if err != nil {
		c.fa.decref()
		err = errors.Annotate(err, "GPIO_GET_LINEEVENT_IOCTL")
		return nil, err
	}

	if err := syscall.SetNonblock(int(req.Fd), true); err != nil {
		c.fa.decref()
		err = errors.Annotate(err, "SetNonblock")
		return nil, err
	}

	le := &lineEvent{
		chip:    c,
		f:       os.NewFile(uintptr(req.Fd), fmt.Sprintf("gpio:event:%d", line)),
		reqFlag: req.RequestFlags,
		events:  req.EventFlags,
		line:    line,
	}
	// runtime.SetFinalizer(le, func(le *lineEvent) { le.Close() })
	return le, nil
}

type lineEvent struct {
	chip    *chip
	f       *os.File
	reqFlag RequestFlag
	events  EventFlag
	line    uint32
	closed  uint32
}

func (self *lineEvent) Close() error {
	if atomic.AddUint32(&self.closed, 1) == 1 {
		// _ = self.f.SetDeadline(time.Time{})
		// _ = syscall.SetNonblock(int(self.f.Fd()), false)
		err := self.f.Close()
		self.chip.fa.decref()
		return err
	}
	return ErrClosed
}

func (self *lineEvent) Read() (byte, error) {
	var data HandleData
	err := RawGetLineValues(int(self.f.Fd()), &data)
	if err != nil {
		err = errors.Annotate(err, "event.Read")
	}
	return data.Values[0], err
}

func (self *lineEvent) Wait(timeout time.Duration) (EventData, error) {
	const tag = "event.Wait"
	var deadline time.Time
	var e EventData
	var err error
	if timeout != 0 {
		deadline = time.Now().Add(timeout)
	}
	if err = self.f.SetDeadline(deadline); err != nil {
		err = errors.Annotate(err, tag)
		return e, err
	}
	e, err = self.readEvent()
	// specifically don't annotate timeout, to ease external code checks
	if err == ErrTimeout {
		return e, err
	}
	if err != nil {
		err = errors.Annotate(err, tag)
	}
	return e, err
}

func (self *lineEvent) readEvent() (EventData, error) {
	// dance around File.Read []byte
	const esz = int(unsafe.Sizeof(EventData{}))
	type eventBuf [esz]byte
	var buf eventBuf
	var e EventData
	var n int
	var err error
	n, err = self.f.Read(buf[:])
	// n, err = f.ReadAt(buf[:], 0)
	if IsTimeout(err) {
		return e, ErrTimeout
	}
	if err != nil {
		return e, err
	}
	if n != esz {
		err = errors.Errorf("readEvent fail n=%d expected=%d", n, esz)
		return e, err
	}
	eb := (*eventBuf)(unsafe.Pointer(&e))
	copy((*eb)[:], buf[:])
	return e, nil
}
