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

type DeviceGeneric struct {
	dev   mdb.Device
	ready msync.Signal
}

var (
	ErrTODO = fmt.Errorf("TODO")
)

func (self *DeviceGeneric) Init(ctx context.Context, address uint8, name string) error {
	self.dev.Init(ctx, address, name, binary.BigEndian)
	self.ready = msync.NewSignal()

	if err := self.CommandReset(ctx); err != nil {
		return err
	}
	time.Sleep(self.dev.DelayNext)
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
