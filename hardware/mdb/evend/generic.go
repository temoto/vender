// Package evend incapsulates common parts of MDB protocol for Evend machine
// devices like conveyor, hopper, cup dispenser, elevator, etc.
package evend

import (
	"bytes"
	"context"
	"encoding/binary"
	"time"

	"github.com/pkg/errors"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
)

const (
	genericPollMiss    = 0x04
	genericPollProblem = 0x08
	genericPollBusy    = 0x50
)

type Generic struct {
	dev mdb.Device
}

func (self *Generic) Init(ctx context.Context, address uint8, name string) error {
	self.dev.Init(ctx, address, name, binary.BigEndian)

	if err := self.dev.NewDoReset().Do(ctx); err != nil {
		return err
	}
	self.dev.Log.Infof("device=%s addr=%02x is working", name, address)
	err := self.dev.DoSetup(ctx)
	return err
}

func (self *Generic) NewErrPollProblem(p mdb.Packet) error {
	return errors.Errorf("device=%s POLL=%02x -> need to ask problem code", self.dev.Name, p.Bytes())
}
func (self *Generic) NewErrPollUnexpected(p mdb.Packet) error { return nil }

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

func (self *Generic) NewPollWait(tag string, timeout time.Duration, ignoreBits byte) engine.Doer {
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
		if value&genericPollProblem != 0 {
			err := self.NewErrPollProblem(r.P)
			// self.dev.Log.Errorf(err.Error())
			// value &^= genericPollError
			return true, err
		}
		// self.dev.Log.Debugf("npw v=%02x i=%02x &=%02x", value, ignoreBits, value&^ignoreBits)
		if value&^ignoreBits == 0 {
			return false, nil
		}
		return true, self.NewErrPollUnexpected(r.P)
	}
	return self.dev.NewPollLoopActive(tag, timeout, fun)
}

// mixer/elevator POLL returns 2 bytes
func (self *Generic) NewPollWait2(tag string, timeout time.Duration) engine.Doer {
	success := []byte{0x0d, 0x00}
	fun := func(r mdb.PacketError) (bool, error) {
		if r.E != nil {
			return true, r.E
		}
		bs := r.P.Bytes()
		if len(bs) == 0 {
			self.dev.Log.Debugf("device=%s POLL=empty", self.dev.Name)
			return false, nil
			// return true, errors.Errorf("device=%s POLL=%02x -> expected non-empty", self.dev.Name, bs)
		}
		if bytes.Equal(bs, success) {
			return true, nil
		}
		if bs[0] == 0x04 {
			return true, errors.Errorf("device=%s POLL=%02x -> parsed error", self.dev.Name, bs)
		}
		return true, self.NewErrPollUnexpected(r.P)
	}
	return self.dev.NewPollLoopActive(tag, timeout, fun)
}
