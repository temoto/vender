package mdb

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/log2"
)

const (
	DefaultDelayErr      = 500 * time.Millisecond
	DefaultDelayNext     = 200 * time.Millisecond
	DefaultDelayIdle     = 700 * time.Millisecond
	DefaultIdleThreshold = 30 * time.Second
)

type Device struct {
	lk     sync.Mutex
	mdber  *mdb
	pollCh chan Packet

	Log           *log2.Log
	Address       uint8
	Name          string
	ByteOrder     binary.ByteOrder
	DelayNext     time.Duration
	DelayErr      time.Duration
	DelayIdle     time.Duration
	IdleThreshold time.Duration
	PacketReset   Packet
	PacketSetup   Packet
	PacketPoll    Packet

	SetupResponse Packet
}

func (self *Device) Init(ctx context.Context, addr uint8, name string, byteOrder binary.ByteOrder) {
	self.lk.Lock()
	defer self.lk.Unlock()

	self.Log = log2.ContextValueLogger(ctx, log2.ContextKey)
	mdber := ContextValueMdber(ctx, ContextKey)
	self.mdber = mdber
	self.pollCh = make(chan Packet, 1)

	self.Address = addr
	self.Name = name
	self.ByteOrder = byteOrder
	self.DelayNext = DefaultDelayNext
	self.DelayErr = DefaultDelayErr
	self.DelayIdle = DefaultDelayIdle
	self.IdleThreshold = DefaultIdleThreshold
	self.SetupResponse = Packet{}
	self.PacketReset = MustPacketFromBytes([]byte{self.Address + 0}, true)
	self.PacketSetup = MustPacketFromBytes([]byte{self.Address + 1}, true)
	self.PacketPoll = MustPacketFromBytes([]byte{self.Address + 3}, true)
}

func (self *Device) Tx(request Packet) (r PacketError) {
	r.E = self.mdber.Tx(request, &r.P)
	return
}

func (self *Device) NewDoTx(request Packet) (*DoRequest, <-chan PacketError) {
	d := &DoRequest{
		dev:     self,
		request: request,
		rch:     make(chan PacketError, 1),
	}
	return d, d.rch
}
func (self *Device) NewDoTxNR(request Packet) *DoRequest {
	d := &DoRequest{dev: self, request: request}
	return d
}

func (self *Device) NewDoReset() engine.Doer { return self.NewDoTxNR(self.PacketReset) }
func (self *Device) DoSetup(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	r := self.Tx(self.PacketSetup)
	if r.E != nil {
		self.Log.Errorf("device=%s mdb request=%s err=%v", self.Name, self.PacketSetup.Format(), r.E)
		return r.E
	}
	self.SetupResponse = r.P
	self.Log.Debugf("device=%s SetupResponse=(%d)%s", self.Name, self.SetupResponse.Len(), self.SetupResponse.Format())
	return nil
}

type PollParseFunc func(PacketError) bool

// "idle mode" checking, wants to run forever
// used by coin/bill devices
func (self *Device) PollLoopPassive(ctx context.Context, a *alive.Alive, fun PollParseFunc) {
	lastActive := time.Now()
	stopch := a.StopChan()
	delay := self.DelayNext
	delayTimer := time.NewTimer(delay)
	delayTimer.Stop()
	r := PacketError{}

	for a.IsRunning() {
		r = self.DoPollSync(ctx)
		parsedActive := fun(r)
		if r.E != nil {
			delay = self.DelayErr
			lastActive = time.Now()
		} else {
			if parsedActive {
				delay = self.DelayNext
				lastActive = time.Now()
			} else if delay != self.DelayIdle {
				if time.Now().Sub(lastActive) > self.IdleThreshold {
					delay = self.DelayIdle
				}
			}
		}
		if !delayTimer.Stop() {
			<-delayTimer.C
		}
		delayTimer.Reset(delay)
		select {
		case <-delayTimer.C:
		case <-stopch:
			return
		}
	}
}

type PollActiveFunc func(PacketError) (bool, error)

func (self *Device) NewPollLoopActive(tag string, timeout time.Duration, fun PollActiveFunc) engine.Doer {
	return engine.Func{Name: tag + "-active-poll", F: func(ctx context.Context) error {
		r := PacketError{}
		deadline := time.Now().Add(timeout)

		for {
			r = self.DoPollSync(ctx)
			stop, err := fun(r)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
			time.Sleep(self.DelayNext)
			if time.Now().After(deadline) {
				return errors.Timeoutf(tag)
			}
		}
	}}
}

func (self *Device) NewPollUntilEmpty(tag string, timeout time.Duration, ignore []Packet) engine.Doer {
	fun := func(r PacketError) (bool, error) {
		if r.E != nil {
			return false, r.E
		}
		if r.P.Len() == 0 {
			return true, nil
		}
		for _, x := range ignore {
			if r.P.Equal(&x) {
				return false, nil
			}
		}
		return false, errors.Errorf("%s poll-until unexpected response=%02x", tag, r.P.Bytes())
	}
	return self.NewPollLoopActive(tag, timeout, fun)
}

func (self *Device) DoPollSync(ctx context.Context) PacketError {
	// self.lk.Lock()
	// defer self.lk.Unlock()
	r := self.Tx(self.PacketPoll)
	if r.E != nil {
		self.Log.Errorf("device=%s POLL err=%v", self.Name, r.E)
	} else {
		self.Log.Debugf("device=%s POLL=(%d)%s", self.Name, r.P.Len(), r.P.Format())
	}
	return r
}

type PacketError struct {
	E error
	P Packet
}

// Doer wrap for mbder.Tx()
type DoRequest struct {
	dev     *Device
	request Packet
	rch     chan PacketError
}

func (self *DoRequest) Do(ctx context.Context) error {
	r := self.dev.Tx(self.request)
	if self.rch != nil {
		self.rch <- r
	}
	return r.E
}
func (self *DoRequest) String() string {
	return "mdb=" + self.request.Format()
}

type DoPoll struct {
	Dev *Device
}

func (self *DoPoll) Do(ctx context.Context) error { return nil }
func (self *DoPoll) String() string               { return "TODO" }
