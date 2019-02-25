// Package evend incapsulates common parts of MDB protocol for Evend machine
// devices like conveyor, hopper, cup dispenser, elevator, etc.
package evend

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
)

const (
	genericPollMiss  = 0x04
	genericPollError = 0x08
	genericPollBusy  = 0x50
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

func (self *Generic) CommandErrorCode(ctx context.Context) (byte, error) {
	bs := []byte{self.dev.Address + 4, 0x02}
	request := mdb.MustPacketFromBytes(bs, true)
	r := self.dev.Tx(request)
	if r.E != nil {
		self.dev.Log.Errorf("device=%s request=%s response=%s", self.dev.Name, request.Format(), r.E)
		return 0, r.E
	}
	rs := r.P.Bytes()
	if len(rs) < 1 {
		err := errors.Errorf("device=%s request=%s response=%s", self.dev.Name, request.Format(), r.E)
		return 0, err
	}
	return rs[0], nil
}

func (self *Generic) NewWait(tag string, timeout time.Duration, while ...byte) engine.Doer {
	fun := func(r mdb.PacketError) (bool, error) {
		if r.E != nil {
			return true, r.E
		}
		bs := r.P.Bytes()
		if len(bs) == 0 {
			return true, nil
		}
		if len(bs) > 1 {
			return true, errors.Errorf("device=%s POLL=%02x -> too long", self.dev.Name, bs)
		}
		value := bs[0]
		if value&genericPollError != 0 {
			self.dev.Log.Errorf("device=%s POLL=%02x -> need errcode", self.dev.Name, bs)
			value &^= genericPollError
		}
		for _, b := range while {
			if value&b == b {
				return true, nil
			}
		}
		return false, nil
	}
	return self.dev.NewPollLoopActive(tag, timeout, fun)
}
