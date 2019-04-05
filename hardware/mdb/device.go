package mdb

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/log2"
)

const (
	DefaultDelayIdle     = 700 * time.Millisecond
	DefaultDelayNext     = 200 * time.Millisecond
	DefaultDelayReset    = 500 * time.Millisecond
	DefaultIdleThreshold = 30 * time.Second
)

type Device struct {
	cmdLk   sync.Mutex // TODO explore if chan approach is better
	txfun   TxFunc
	lastOff time.Time // unused yet TODO self.lastOff.IsZero()

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
}

func (self *Device) Tx(request Packet) (r PacketError) {
	r.E = self.txfun(request, &r.P)
	if r.E == nil {
		self.lastOff = time.Time{}
	} else {
		self.Log.Errorf("mdb.dev.%s request=%s err=%v", self.Name, request.Format(), r.E)
		if self.lastOff.IsZero() && errors.Cause(r.E) == ErrTimeout {
			self.lastOff = time.Now()
		}
		r.E = errors.Annotatef(r.E, "request=%x", request.Bytes())
	}
	return
}

func (self *Device) NewTx(request Packet) *DoRequest {
	return &DoRequest{dev: self, request: request}
}

func (self *Device) NewReset() engine.Doer {
	tag := fmt.Sprintf("mdb.%s.reset", self.Name)

	return engine.Func0{Name: tag, F: func() error {
		self.cmdLk.Lock()
		defer self.cmdLk.Unlock()

		request := self.PacketReset
		r := self.Tx(request)
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
	}}
}

func (self *Device) DoSetup(ctx context.Context) error {
	self.cmdLk.Lock()
	defer self.cmdLk.Unlock()

	request := self.PacketSetup
	r := self.Tx(request)
	if r.E != nil {
		return r.E
	}
	self.SetupResponse = r.P
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
			if err != nil {
				return errors.Annotate(err, tag)
			}
			if stop || timeout == 0 {
				return nil
			}
			time.Sleep(self.DelayNext)
			if time.Since(tbegin) > timeout {
				return errors.Timeoutf(tag)
			}
		}
	}}
}

type PacketError struct {
	E error
	P Packet
}

// Doer wrap for mbder.Tx()
type DoRequest struct {
	dev     *Device
	request Packet
}

func (self *DoRequest) Do(ctx context.Context) error {
	r := self.dev.Tx(self.request)
	return r.E
}
func (self *DoRequest) String() string {
	return fmt.Sprintf("mdb.%s/%s", self.dev.Name, self.request.Format())
}

type DoPoll struct {
	Dev *Device
}

func (self *DoPoll) Do(ctx context.Context) error { return nil }
func (self *DoPoll) String() string               { return "TODO" }
