// Package evend incapsulates common parts of MDB protocol for Evend machine
// devices like conveyor, hopper, cup dispenser, elevator, etc.
package evend

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
)

// Mostly affects POLL response, see doc.
type evendProtocol uint8

const proto1 evendProtocol = 1
const proto2 evendProtocol = 2

const (
	genericPollMiss    = 0x04
	genericPollProblem = 0x08
	genericPollBusy    = 0x50

	DefaultReadyTimeout = 5 * time.Second
)

type Generic struct {
	dev          mdb.Device
	proto        evendProtocol
	logPrefix    string
	readyTimeout time.Duration

	// For most devices 0x50 = busy
	// valve 0x10 = busy, 0x40 = hot water is colder than configured
	proto2BusyMask   byte
	proto2IgnoreMask byte
}

func (self *Generic) Init(ctx context.Context, address uint8, name string, proto evendProtocol) error {
	if self.proto2BusyMask == 0 {
		self.proto2BusyMask = genericPollBusy
	}
	if self.readyTimeout == 0 {
		self.readyTimeout = DefaultReadyTimeout
	}
	self.proto = proto
	self.logPrefix = fmt.Sprintf("device=%s(%02x) proto%v", name, address, proto)

	if self.dev.DelayReset == 0 {
		self.dev.DelayReset = 2100 * time.Millisecond
	}
	self.dev.Init(ctx, address, name, binary.BigEndian)

	if err := self.dev.NewDoReset().Do(ctx); err != nil {
		return err
	}
	err := self.dev.DoSetup(ctx)
	return err
}

func (self *Generic) NewErrPollProblem(p mdb.Packet) error {
	return errors.Errorf("%s POLL=%x -> need to ask problem code", self.logPrefix, p.Bytes())
}
func (self *Generic) NewErrPollUnexpected(p mdb.Packet) error {
	return errors.Errorf("%s POLL=%x unexpected", self.logPrefix, p.Bytes())
}

func (self *Generic) CommandAction(args []byte) error {
	bs := make([]byte, len(args)+1)
	bs[0] = self.dev.Address + 2
	copy(bs[1:], args)
	request := mdb.MustPacketFromBytes(bs, true)
	r := self.dev.Tx(request)
	if r.E != nil {
		self.dev.Log.Errorf("device=%s mdb request=%s err=%v", self.dev.Name, request.Format(), r.E)
		return r.E
	}
	self.dev.Log.Debugf("device=%s action=%x response=(%d)%s", self.dev.Name, args, r.P.Len(), r.P.Format())
	return nil
}

func (self *Generic) CommandErrorCode() (byte, error) {
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

// proto1/2 agnostic, use it before action
func (self *Generic) DoWaitReady(tagPrefix string) engine.Doer {
	tag := tagPrefix + "/wait-ready"
	switch self.proto {
	case proto1:
		return self.dev.NewPollUntilEmpty(tag, self.readyTimeout, nil)
	case proto2:
		return self.doProto2PollWait(tag, self.readyTimeout, genericPollMiss)
	default:
		panic("code error")
	}
}

// proto1/2 agnostic, use it after action
func (self *Generic) DoWaitDone(tagPrefix string, timeout time.Duration) engine.Doer {
	tag := tagPrefix + "/wait-done"
	switch self.proto {
	case proto1:
		return self.doProto1PollWaitSuccess(tag, timeout)
	case proto2:
		return self.doProto2PollWait(tag, timeout, self.proto2BusyMask)
	default:
		panic("code error")
	}
}

func (self *Generic) doProto1PollWaitSuccess(tag string, timeout time.Duration) engine.Doer {
	success := []byte{0x0d, 0x00}
	fun := func(r mdb.PacketError) (bool, error) {
		if r.E != nil {
			return true, r.E
		}
		bs := r.P.Bytes()
		if len(bs) == 0 {
			self.dev.Log.Debugf("device=%s POLL=empty", self.dev.Name)
			return false, nil
			// return true, errors.Errorf("device=%s POLL=%x -> expected non-empty", self.dev.Name, bs)
		}
		if bytes.Equal(bs, success) {
			return true, nil
		}
		if bs[0] == 0x04 {
			return true, errors.Errorf("device=%s POLL=%x -> parsed error", self.dev.Name, bs)
		}
		return true, self.NewErrPollUnexpected(r.P)
	}
	return self.dev.NewPollLoopActive(tag, timeout, fun)
}

func (self *Generic) doProto2PollWait(tag string, timeout time.Duration, ignoreBits byte) engine.Doer {
	fun := func(r mdb.PacketError) (bool, error) {
		if r.E != nil {
			return true, r.E
		}
		bs := r.P.Bytes()
		if len(bs) == 0 {
			return true, nil
		}
		if len(bs) > 1 {
			return true, errors.Errorf("%s POLL=%x -> too long", self.logPrefix, bs)
		}
		value := bs[0]
		value &^= self.proto2IgnoreMask
		if (value&^ignoreBits)&genericPollMiss != 0 {
			// FIXME
			// 04 during WaitReady is "OK, poll few more"
			// 04 during WaitDone is "oops, device reboot in operation"
			return true, errors.Errorf("%s POLL=%x continous connection lost, (TODO decide reset?)", self.logPrefix, bs)
		}
		if value&genericPollProblem != 0 {
			// err := self.NewErrPollProblem(r.P)
			// self.dev.Log.Errorf(err.Error())
			value &^= genericPollProblem
			errCode, err := self.CommandErrorCode()
			if err == nil {
				err = errors.Errorf("%s POLL=%x errorcode=%[3]d %[3]02x", self.logPrefix, bs, errCode)
			}
			return true, err
		}
		self.dev.Log.Debugf("npw v=%02x i=%02x &=%02x", value, ignoreBits, value&^ignoreBits)
		if value&^ignoreBits == 0 {
			return false, nil
		}
		return true, self.NewErrPollUnexpected(r.P)
	}
	return self.dev.NewPollLoopActive(tag, timeout, fun)
}
