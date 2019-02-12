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

	// setupResponse []byte
	//
	// packetReset mdb.Packet
	// packetSetup mdb.Packet
	// packetPoll  mdb.Packet
}

var (
	ErrTODO = fmt.Errorf("TODO")
)

func (self *DeviceGeneric) Init(ctx context.Context, address uint8, name string) error {
	self.dev.Init(ctx, address, name, binary.BigEndian)
	self.ready = msync.NewSignal()
	// self.setupResponse = make([]byte, 0, mdb.PacketMaxLength)
	// self.packetReset = mdb.MustPacketFromBytes([]byte{self.dev.Address + 0}, true)
	// self.packetSetup = mdb.MustPacketFromBytes([]byte{self.dev.Address + 1}, true)
	// self.packetPoll = mdb.MustPacketFromBytes([]byte{self.dev.Address + 3}, true)

	if err := self.CommandReset(ctx); err != nil {
		return err
	}
	// FIXME magic number
	time.Sleep(200 * time.Millisecond)
	_, err := self.CommandSetup(ctx)
	return err
}

func (self *DeviceGeneric) ReadyChan() <-chan msync.Nothing {
	return self.ready
}

func (self *DeviceGeneric) CommandReset(ctx context.Context) error {
	return self.dev.NewDoReset().Do(ctx)
}

func (self *DeviceGeneric) CommandSetup(ctx context.Context) ([]byte, error) {
	err := self.dev.DoSetup(ctx)
	return self.dev.SetupResponse.Bytes(), err
}

func (self *DeviceGeneric) CommandAction(ctx context.Context, args []byte) error {
	bs := make([]byte, len(args)+1)
	bs[0] = self.dev.Address + 2
	copy(bs[1:], args)
	request := mdb.MustPacketFromBytes(bs, true)
	r := self.dev.Tx(request)
	if r.E != nil {
		log.Printf("device=%s mdb request=%s err=%v", self.dev.Name, request.Format(), r.E)
		return r.E
	}
	log.Printf("device=%s setup response=(%d)%s", self.dev.Name, r.P.Len(), r.P.Format())
	return nil
}

func (self *DeviceGeneric) CommandPoll(ctx context.Context) error {
	err := self.dev.DoPoll(ctx)
	if err != nil {
		return err
	}
	result := <-self.dev.PollChan()
	log.Printf("device=%s poll response=%s", self.dev.Name, result.Format())
	if result.Len() == 0 {
		self.ready.Set()
	}
	return nil
}
