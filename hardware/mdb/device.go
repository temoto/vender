package mdb

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/temoto/errors"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/log2"
)

const ErrCodeNone int32 = -1

const (
	DefaultDelayIdle     = 700 * time.Millisecond
	DefaultDelayNext     = 200 * time.Millisecond
	DefaultDelayReset    = 500 * time.Millisecond
	DefaultIdleThreshold = 30 * time.Second
)

type PacketError struct {
	E error
	P Packet
}

type Device struct { //nolint:maligned
	cmdLk   sync.Mutex // TODO explore if chan approach is better
	txfun   TxFunc
	lastOff time.Time
	errCode int32

	// "ready for useful work", with RESET, configure, calibration done.
	ready uint32 // atomic bool

	Log           *log2.Log
	Address       uint8
	Name          string
	ByteOrder     binary.ByteOrder
	DelayIdle     time.Duration
	DelayNext     time.Duration
	DelayReset    time.Duration
	IdleThreshold time.Duration
	PacketReset   Packet
	PacketSetup   Packet
	PacketPoll    Packet
	DoReset       engine.Doer

	SetupResponse Packet
}

func (self *Device) Init(txfun TxFunc, log *log2.Log, addr uint8, name string, byteOrder binary.ByteOrder) {
	self.cmdLk.Lock()
	defer self.cmdLk.Unlock()

	self.Address = addr
	self.ByteOrder = byteOrder
	self.Log = log
	self.txfun = txfun
	self.Name = name
	self.errCode = ErrCodeNone

	if self.DelayIdle == 0 {
		self.DelayIdle = DefaultDelayIdle
	}
	if self.DelayNext == 0 {
		self.DelayNext = DefaultDelayNext
	}
	if self.DelayReset == 0 {
		self.DelayReset = DefaultDelayReset
	}
	if self.IdleThreshold == 0 {
		self.IdleThreshold = DefaultIdleThreshold
	}
	self.SetupResponse = Packet{}
	self.PacketReset = MustPacketFromBytes([]byte{self.Address + 0}, true)
	self.PacketSetup = MustPacketFromBytes([]byte{self.Address + 1}, true)
	self.PacketPoll = MustPacketFromBytes([]byte{self.Address + 3}, true)
	self.DoReset = engine.Func0{Name: fmt.Sprintf("mdb.%s.reset", self.Name), F: self.Reset}
}

func (self *Device) ValidateErrorCode() error {
	value := atomic.LoadInt32(&self.errCode)
	if value == ErrCodeNone {
		return nil
	}
	return errors.Errorf("mdb.%s unhandled errorcode=%d", self.Name, value)
}

func (self *Device) ValidateOnline() error {
	if self.lastOff.IsZero() {
		return nil
	}
	return errors.Errorf("mdb.%s offline duration=%v", self.Name, time.Since(self.lastOff))
}

func (self *Device) Tx(request Packet) (r PacketError) {
	if ee := self.ValidateErrorCode(); ee != nil {
		// self.Log.Errorf("TODO-ERRCODE %v", errors.ErrorStack(errors.Trace(ee)))
		// TODO self.Reset()
	}
	return self.tx(request)
}

func (self *Device) tx(request Packet) (r PacketError) {
	r.E = self.txfun(request, &r.P)
	if r.E == nil {
		self.lastOff = time.Time{}
	} else {
		self.Log.Errorf("mdb.%s request=%s err=%v", self.Name, request.Format(), r.E)
		if self.lastOff.IsZero() && errors.Cause(r.E) == ErrTimeout {
			self.lastOff = time.Now()
		}
		r.E = errors.Annotatef(r.E, "request=%x", request.Bytes())
	}
	return
}

func (self *Device) DoSetup(ctx context.Context) error {
	self.cmdLk.Lock()
	defer self.cmdLk.Unlock()

	request := self.PacketSetup
	r := self.tx(request)
	if r.E != nil {
		return r.E
	}
	self.SetupResponse = r.P
	return nil
}

func (self *Device) ErrorCode() int32 { return atomic.LoadInt32(&self.errCode) }
func (self *Device) SetErrorCode(c int32) {
	prev := atomic.SwapInt32(&self.errCode, c)
	if prev != ErrCodeNone {
		self.Log.Errorf("mdb.%s CRITICAL SetErrorCode overwrite previous=%d", self.Name, prev)
		// TODO tele
	}
	if c != ErrCodeNone {
		// self.SetReady(false)
		self.Log.Errorf("mdb.%s errcode=%d", self.Name, c)
		// TODO tele
	}
}

func (self *Device) Ready() bool { return atomic.LoadUint32(&self.ready) == 1 }
func (self *Device) SetReady(r bool) {
	u := uint32(0)
	if r {
		u = 1
	}
	atomic.StoreUint32(&self.ready, u)
}

func (self *Device) Reset() error {
	self.cmdLk.Lock()
	defer self.cmdLk.Unlock()
	return self.locked_reset()
}

// cmdLk used to ensure no concurrent commands between tx() and Sleep()
func (self *Device) locked_reset() error {
	tag := fmt.Sprintf("mdb.%s.reset", self.Name)
	request := self.PacketReset
	r := self.tx(request)
	atomic.StoreInt32(&self.errCode, ErrCodeNone)
	self.SetReady(false)
	if r.E != nil {
		if errors.Cause(r.E) == ErrTimeout {
			self.lastOff = time.Now()
			self.Log.Errorf("%s addr=%02x is offline RESET err=timeout", tag, self.Address)
		} else {
			self.Log.Errorf("%s RESET err=%s", tag, errors.ErrorStack(r.E))
		}
		return r.E
	}
	self.Log.Infof("%s addr=%02x is working", tag, self.Address)
	time.Sleep(self.DelayReset)
	return nil
}

// "Idle mode" polling, runs forever until receive on `stopch`.
// Switches between fast/idle delays.
// Used by bill/coin devices.
type PollDelay struct {
	lastActive time.Time
	lastDelay  time.Duration
}

func (self *PollDelay) Delay(dev *Device, active bool, err bool, stopch <-chan struct{}) bool {
	delay := dev.DelayNext

	if err {
		delay = dev.DelayIdle
	} else if active {
		self.lastActive = time.Now()
	} else if self.lastDelay != dev.DelayIdle { // save time syscall while idle continues
		if time.Since(self.lastActive) > dev.IdleThreshold {
			delay = dev.DelayIdle
		}
	}
	self.lastDelay = delay

	select {
	case <-stopch:
		return false
	case <-time.After(delay):
		return true
	}
}

type PollFunc func(Packet) (stop bool, err error)

func (self *Device) NewPollLoop(tag string, request Packet, timeout time.Duration, fun PollFunc) engine.Doer {
	tag += "/poll-loop"
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		var r PacketError
		tbegin := time.Now()

		self.cmdLk.Lock()
		defer self.cmdLk.Unlock()
		for {
			r = self.Tx(request)
			if r.E != nil {
				return errors.Annotate(r.E, tag)
			}
			stop, err := fun(r.P)
			if err == nil && stop { // success
				return nil
			} else if err == nil && !stop { // try again
				if timeout == 0 {
					return errors.Errorf("tag=%s timeout=0 invalid", tag)
				}
				time.Sleep(self.DelayNext)
				if time.Since(tbegin) > timeout {
					return errors.Timeoutf(tag)
				}
				continue
			}

			return errors.Annotate(err, tag)
		}
	}}
}
