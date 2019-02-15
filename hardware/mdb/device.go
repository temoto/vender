package mdb

import (
	"context"
	"encoding/binary"
	"log"
	"sync"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/vender/engine"
)

const (
	DefaultDelayErr      = 500 * time.Millisecond
	DefaultDelayNext     = 200 * time.Millisecond
	DefaultDelayIdle     = 700 * time.Millisecond
	DefaultIdleThreshold = 30 * time.Second
)

type Device struct {
	lk     sync.Mutex
	mdber  Mdber
	pollCh chan Packet

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
		log.Printf("device=%s mdb request=%s err=%v", self.Name, self.PacketSetup.Format(), r.E)
		return r.E
	}
	self.SetupResponse = r.P
	log.Printf("device=%s SetupResponse=(%d)%s", self.Name, self.SetupResponse.Len(), self.SetupResponse.Format())
	return nil
}

type PollParseFunc func(PacketError)

func (self *Device) PollLoop(ctx context.Context, a *alive.Alive, fun PollParseFunc) {
	lastActive := time.Now()
	stopch := a.StopChan()
	delay := self.DelayNext
	delayTimer := time.NewTimer(delay)
	delayTimer.Stop()
	r := PacketError{}

	for a.IsRunning() {
		r = self.DoPollSync(ctx)
		fun(r)
		if r.E != nil {
			delay = self.DelayErr
		}

		now := time.Now()
		// FIXME implicitly encodes "empty POLL result = inactive"
		// which may not be true for some devices
		// TODO better way to learn active/idle from parser func
		if r.P.Len() > 0 {
			lastActive = now
		} else {
			if r.E == nil && now.Sub(lastActive) > self.IdleThreshold {
				delay = self.DelayIdle
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

func (self *Device) DoPollSync(ctx context.Context) PacketError {
	// self.lk.Lock()
	// defer self.lk.Unlock()
	r := self.Tx(self.PacketPoll)
	if r.E != nil {
		log.Printf("device=%s POLL err=%v", self.Name, r.E)
	} else {
		log.Printf("device=%s POLL=(%d)%s", self.Name, r.P.Len(), r.P.Format())
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
