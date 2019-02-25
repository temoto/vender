// Package evend incapsulates common parts of MDB protocol for Evend machine
// devices like conveyor, hopper, cup dispenser, elevator, etc.
package evend

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/temoto/vender/hardware/mdb"
)

type Generic struct {
	dev mdb.Device
}

var (
	ErrTODO = fmt.Errorf("TODO")
)

func (self *Generic) Init(ctx context.Context, address uint8, name string) error {
	self.dev.Init(ctx, address, name, binary.BigEndian)

	if err := self.CommandReset(ctx); err != nil {
		return err
	}
	time.Sleep(self.dev.DelayNext)
	_, err := self.CommandSetup(ctx)
	return err
}

func (self *Generic) CommandReset(ctx context.Context) error {
	return self.dev.NewDoReset().Do(ctx)
}

func (self *Generic) CommandSetup(ctx context.Context) ([]byte, error) {
	err := self.dev.DoSetup(ctx)
	return self.dev.SetupResponse.Bytes(), err
}

func (self *Generic) CommandAction(ctx context.Context, args []byte) error {
	bs := make([]byte, len(args)+1)
	bs[0] = self.dev.Address + 2
	copy(bs[1:], args)
	request := mdb.MustPacketFromBytes(bs, true)
	r := self.dev.Tx(request)
	if r.E != nil {
		self.dev.Log.Errorf("device=%s mdb request=%s err=%v", self.dev.Name, request.Format(), r.E)
		return r.E
	}
	self.dev.Log.Debugf("device=%s action=%02x response=(%d)%s", self.dev.Name, args, r.P.Len(), r.P.Format())
	return nil
}
