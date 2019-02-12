package mdb

import (
	"context"
	"encoding/binary"
	"log"
	"sync"

	"github.com/temoto/vender/helpers/msync"
)

type Device struct {
	lk     sync.Mutex
	mdber  Mdber
	pollCh chan Packet

	Address     uint8
	Name        string
	ByteOrder   binary.ByteOrder
	PacketReset Packet
	PacketSetup Packet
	PacketPoll  Packet

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

func (self *Device) NewDoReset() msync.Doer { return self.NewDoTxNR(self.PacketReset) }
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

func (self *Device) DoPollSync(ctx context.Context) PacketError {
	// self.lk.Lock()
	// defer self.lk.Unlock()
	r := self.Tx(self.PacketPoll)
	return r
}

// Iff DoPoll() returns nil then you can read result from self.PollChan()
func (self *Device) DoPoll(ctx context.Context) error {
	r := self.DoPollSync(ctx)
	if r.E != nil {
		// TODO
		return r.E
	}
	select {
	case self.pollCh <- r.P:
	default:
		log.Printf("CRITICAL mdb.DoPoll chan overflow, read old value first, response=%s", r.P.Format())
	}
	return nil
}
func (self *Device) PollChan() <-chan Packet { return self.pollCh }

// func (self *Device) DebugDo(parent *msync.Node, request Packet) PacketError {
// 	d, rch := self.NewDoTx(request)
// 	parent.Append(d)
// 	return <-rch
// }

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
