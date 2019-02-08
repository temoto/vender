// Package evend incapsulates common parts of MDB protocol for Evend machine
// devices like conveyor, hopper, cup dispenser, elevator, etc.
package evend

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers/msync"
)

const (
	DelayErr  = 500 * time.Millisecond
	DelayNext = 200 * time.Millisecond
)

type DeviceGeneric struct {
	dev   mdb.Device
	ready msync.Signal

	setupResponse []byte

	packetReset  mdb.Packet
	packetSetup  mdb.Packet
	packetAction mdb.Packet
	packetPoll   mdb.Packet
}

var (
	ErrTODO = fmt.Errorf("TODO")
)

func (self *DeviceGeneric) Init(ctx context.Context, mdber mdb.Mdber, address uint8, name string) error {
	// TODO read config
	self.dev.Address = address
	self.dev.ByteOrder = binary.BigEndian
	self.dev.Name = name
	self.dev.Mdber = mdber
	self.ready = msync.NewSignal()
	self.setupResponse = make([]byte, 0, mdb.PacketMaxLength)
	self.packetReset = mdb.MustPacketFromBytes([]byte{self.dev.Address + 0}, true)
	self.packetSetup = mdb.MustPacketFromBytes([]byte{self.dev.Address + 1}, true)
	self.packetPoll = mdb.MustPacketFromBytes([]byte{self.dev.Address + 3}, true)

	if err := self.CommandReset(); err != nil {
		return err
	}
	// FIXME magic number
	time.Sleep(200 * time.Millisecond)
	_, err := self.CommandSetup()
	return err
}

func (self *DeviceGeneric) ReadyChan() <-chan msync.Nothing {
	return self.ready
}

func (self *DeviceGeneric) CommandReset() error {
	return self.dev.Tx(self.packetReset).E
}

func (self *DeviceGeneric) CommandSetup() ([]byte, error) {
	r := self.dev.Tx(self.packetSetup)
	if r.E != nil {
		log.Printf("device=%s mdb request=%s err=%v", self.dev.Name, self.packetSetup.Format(), r.E)
		return nil, r.E
	}
	self.setupResponse = append(self.setupResponse[:0], r.P.Bytes()...)
	log.Printf("device=%s setup response=(%d)%s", self.dev.Name, r.P.Len(), r.P.Format())
	return self.setupResponse, nil
}

func (self *DeviceGeneric) CommandAction(args []byte) error {
	bs := make([]byte, len(args)+1)
	bs[0] = self.dev.Address + 2
	copy(bs[1:], args)
	request := mdb.MustPacketFromBytes(bs, true)
	r := self.dev.Tx(request)
	if r.E != nil {
		log.Printf("device=%s mdb request=%s err=%v", self.dev.Name, self.packetSetup.Format(), r.E)
		return r.E
	}
	log.Printf("device=%s setup response=(%d)%s", self.dev.Name, r.P.Len(), r.P.Format())
	return nil
}

func (self *DeviceGeneric) CommandPoll() error {
	// now := time.Now()
	r := self.dev.Tx(self.packetPoll)
	log.Printf("device=%s poll response=%s", self.dev.Name, r.P.Format())
	bs := r.P.Bytes()
	if len(bs) == 0 {
		self.ready.Set()
		return nil
	}
	return r.E
}
