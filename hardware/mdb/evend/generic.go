// Package evend incapsulates common parts of MDB protocol to Device
// devices like conveyor, hopper, cup dispenser, elevator, etc.
package evend

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers/msync"
)

const (
	DelayErr  = 500 * time.Millisecond
	DelayNext = 200 * time.Millisecond
)

type DeviceGeneric struct {
	mdb       mdb.Mdber
	address   uint8
	name      string
	byteOrder binary.ByteOrder
	batch     sync.Mutex
	ready     msync.Signal

	setupResponse []byte

	packetReset  *mdb.Packet
	packetSetup  *mdb.Packet
	packetAction *mdb.Packet
	packetPoll   *mdb.Packet
}

var (
	ErrTODO = fmt.Errorf("TODO")
)

// usage: defer x.Batch()()
func (self *DeviceGeneric) Batch() func() {
	self.batch.Lock()
	return self.batch.Unlock
}

func (self *DeviceGeneric) Init(ctx context.Context, mdber mdb.Mdber, address uint8, name string) error {
	// TODO read config
	self.address = address
	self.byteOrder = binary.BigEndian
	self.name = name
	self.mdb = mdber
	self.ready = msync.NewSignal()
	self.setupResponse = make([]byte, 0, mdb.PacketMaxLength)
	self.packetReset = mdb.PacketFromBytes([]byte{self.address + 0})
	self.packetSetup = mdb.PacketFromBytes([]byte{self.address + 1})
	self.packetPoll = mdb.PacketFromBytes([]byte{self.address + 3})

	if err := self.CommandReset(); err != nil {
		return err
	}
	time.Sleep(200 * time.Millisecond)
	_, err := self.CommandSetup()
	return err
}

func (self *DeviceGeneric) ReadyChan() <-chan msync.Nothing {
	return self.ready
}

func (self *DeviceGeneric) CommandReset() error {
	return self.mdb.Tx(self.packetReset, new(mdb.Packet))
}

func (self *DeviceGeneric) CommandSetup() ([]byte, error) {
	response := new(mdb.Packet)
	err := self.mdb.Tx(self.packetSetup, response)
	if err != nil {
		log.Printf("device=%s mdb request=%s err=%v", self.name, self.packetSetup.Format(), err)
		return nil, err
	}
	self.setupResponse = append(self.setupResponse[:0], response.Bytes()...)
	log.Printf("device=%s setup response=(%d)%s", self.name, response.Len(), response.Format())
	return self.setupResponse, nil
}

func (self *DeviceGeneric) CommandAction(args []byte) error {
	bs := make([]byte, len(args)+1)
	bs[0] = self.address + 2
	copy(bs[1:], args)
	request := mdb.PacketFromBytes(bs)
	response := new(mdb.Packet)
	err := self.mdb.Tx(request, response)
	if err != nil {
		log.Printf("device=%s mdb request=%s err=%v", self.name, self.packetSetup.Format(), err)
		return err
	}
	log.Printf("device=%s setup response=(%d)%s", self.name, response.Len(), response.Format())
	return nil
}

func (self *DeviceGeneric) CommandPoll() error {
	// now := time.Now()
	response := new(mdb.Packet)
	err := self.mdb.Tx(self.packetPoll, response)
	log.Printf("device=%s poll response=%s", self.name, response.Format())
	bs := response.Bytes()
	if len(bs) == 0 {
		self.ready.Set()
		return nil
	}
	// if err != nil {
	// 	return err
	// }
	return err
}
