// Package evend incapsulates common parts of MDB protocol for eVend machine
// devices like conveyor, hopper, cup dispenser, elevator, etc.
package evend

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/state"
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
	DefaultResetDelay   = 2100 * time.Millisecond
)

type DeviceErrorCode byte

func (c DeviceErrorCode) Error() string { return fmt.Sprintf("evend errorcode=%d", c) }

type Generic struct {
	dev          mdb.Device
	logPrefix    string
	readyTimeout time.Duration
	proto        evendProtocol

	// For most devices 0x50 = busy
	// valve 0x10 = busy, 0x40 = hot water is colder than configured
	proto2BusyMask   byte
	proto2IgnoreMask byte
}

func (self *Generic) Init(ctx context.Context, address uint8, name string, proto evendProtocol) {
	self.logPrefix = fmt.Sprintf("mdb.evend.%s(%02x)", name, address)

	if self.proto2BusyMask == 0 {
		self.proto2BusyMask = genericPollBusy
	}
	if self.readyTimeout == 0 {
		self.readyTimeout = DefaultReadyTimeout
	}
	self.proto = proto

	if self.dev.DelayBeforeReset == 0 {
		self.dev.DelayBeforeReset = 2 * DefaultResetDelay
	}
	if self.dev.DelayAfterReset == 0 {
		self.dev.DelayAfterReset = DefaultResetDelay
	}
	g := state.GetGlobal(ctx)
	mdbus, _ := g.Mdb()
	self.dev.Init(mdbus, address, name, binary.BigEndian)
}

// FIXME Enum, remove IO from Init
func (self *Generic) FIXME_initIO(ctx context.Context) error {
	tag := self.logPrefix + ".initIO"
	g := state.GetGlobal(ctx)
	_, err := g.Mdb()
	if err != nil {
		return errors.Annotate(err, tag)
	}
	if err = self.dev.Reset(); err != nil {
		return errors.Annotate(err, tag)
	}
	err = self.dev.TxSetup()
	return errors.Annotate(err, tag)
}

func (self *Generic) NewErrPollProblem(p mdb.Packet) error {
	return errors.Errorf("%s POLL=%x -> need to ask problem code", self.logPrefix, p.Bytes())
}
func (self *Generic) NewErrPollUnexpected(p mdb.Packet) error {
	return errors.Errorf("%s POLL=%x unexpected", self.logPrefix, p.Bytes())
}

func (self *Generic) NewAction(tag string, args ...byte) engine.Doer {
	return engine.Func0{Name: tag, F: func() error {
		return self.txAction(args)
	}}
}
func (self *Generic) txAction(args []byte) error {
	bs := make([]byte, len(args)+1)
	bs[0] = self.dev.Address + 2
	copy(bs[1:], args)
	request := mdb.MustPacketFromBytes(bs, true)
	response := mdb.Packet{}
	err := self.dev.TxMaybe(request, &response) // FIXME check everything and change to TxKnown
	if err != nil {
		return err
	}
	self.dev.Log.Debugf("%s action=%x response=(%d)%s", self.logPrefix, args, response.Len(), response.Format())
	return nil
}

func (self *Generic) Diagnostic() (byte, error) {
	tag := self.logPrefix + ".diagnostic"

	bs := []byte{self.dev.Address + 4, 0x02}
	request := mdb.MustPacketFromBytes(bs, true)
	response := mdb.Packet{}
	// Assumptions:
	// - (Known) all evend devices support diagnostic command +402
	// - (Locked) it's safe to call CommandErrorCode concurrently with other
	err := self.dev.Locked_TxKnown(request, &response)
	if err != nil {
		self.dev.SetError(err)
		return 0, errors.Annotate(err, tag)
	}
	rs := response.Bytes()
	if len(rs) < 1 {
		err = errors.Errorf("%s request=%x response=%x", tag, request.Bytes(), rs)
		self.dev.SetError(err)
		return 0, err
	}
	self.dev.SetErrorCode(int32(rs[0]))
	return rs[0], nil
}

// proto1/2 agnostic, use it before action
func (self *Generic) NewWaitReady(tag string) engine.Doer {
	tag += "/wait-ready"
	switch self.proto {
	case proto1:
		fun := func(p mdb.Packet) (bool, error) {
			bs := p.Bytes()
			switch len(bs) {
			case 0: // success path
				return true, nil

			case 2: // device reported error code
				code := bs[1]
				self.dev.Log.Errorf("%s response=%x errorcode=%d", tag, bs, code)
				self.dev.SetErrorCode(int32(code))
				// self.dev.SetReady(false)
				// TODO tele
				return true, DeviceErrorCode(code)

			default:
				err := errors.Errorf("%s unknown response=%x", tag, bs)
				self.dev.Log.Error(err)
				return false, err
			}
		}
		return self.dev.NewPollLoop(tag, self.dev.PacketPoll, self.readyTimeout, fun)

	case proto2:
		fun := func(p mdb.Packet) (bool, error) {
			bs := p.Bytes()
			// self.dev.Log.Debugf("%s POLL=%x", tag, bs)
			if stop, err := self.proto2PollCommon(tag, bs); stop || err != nil {
				return stop, err
			}
			value := bs[0]
			value &^= self.proto2IgnoreMask

			// 04 during WaitReady is "OK, poll few more"
			if value&genericPollMiss != 0 {
				// self.dev.SetReady(false)
				value &^= genericPollMiss
			}

			// busy during WaitReady is problem (previous action did not finish cleanly)
			if value == self.proto2BusyMask {
				err := errors.Errorf("%s PLEASE REPORT WaitReady POLL=%x (busy) unexpected", tag, bs[0])
				self.dev.SetError(err)
				return false, nil
			}

			if value == 0 {
				// self.dev.Log.Debugf("%s WaitReady value=%02x (%02x&^%02x) -> late repeat", tag, value, bs[0], self.proto2IgnoreMask)
				return false, nil
			}

			self.dev.SetErrorCode(1)
			self.dev.Log.Errorf("%s PLEASE REPORT WaitReady value=%02x (%02x&^%02x) -> unexpected", tag, value, bs[0], self.proto2IgnoreMask)
			return true, self.NewErrPollUnexpected(p)
		}
		return self.dev.NewPollLoop(tag, self.dev.PacketPoll, self.readyTimeout, fun)

	default:
		panic("code error")
	}
}

// proto1/2 agnostic, use it after action
func (self *Generic) NewWaitDone(tag string, timeout time.Duration) engine.Doer {
	tag += "/wait-done"

	switch self.proto {
	case proto1:
		return self.newProto1PollWaitSuccess(tag, timeout)

	case proto2:
		fun := func(p mdb.Packet) (bool, error) {
			bs := p.Bytes()
			// self.dev.Log.Debugf("%s POLL=%x", tag, bs)
			if stop, err := self.proto2PollCommon(tag, bs); stop || err != nil {
				// self.dev.Log.Debugf("%s ... return common stop=%t err=%v", tag, stop, err)
				return stop, err
			}
			value := bs[0]
			value &^= self.proto2IgnoreMask

			// 04 during WaitDone is "oops, device reboot in operation"
			if value&genericPollMiss != 0 {
				self.dev.SetState(mdb.DeviceOnline)
				return true, errors.Errorf("%s POLL=%x ignore=%02x continous connection lost, (TODO decide reset?)", tag, bs, self.proto2IgnoreMask)
			}

			// busy during WaitDone is correct path
			if value == self.proto2BusyMask {
				// self.dev.Log.Debugf("%s POLL=%x (busy) -> ok, repeat", tag, bs[0])
				return false, nil
			}

			self.dev.Log.Debugf("%s poll-wait-done value=%02x (%02x&^%02x)", tag, value, bs[0], self.proto2IgnoreMask)
			if value == 0 {
				self.dev.Log.Debugf("%s PLEASE REPORT POLL=%x final=00", tag, bs[0])
				return true, nil
			}
			return true, self.NewErrPollUnexpected(p)
		}
		return self.dev.NewPollLoop(tag, self.dev.PacketPoll, timeout, fun)

	default:
		panic("code error")
	}
}

func (self *Generic) newProto1PollWaitSuccess(tag string, timeout time.Duration) engine.Doer {
	success := []byte{0x0d, 0x00}
	fun := func(p mdb.Packet) (bool, error) {
		bs := p.Bytes()
		if len(bs) == 0 { // empty -> try again
			// self.dev.Log.Debugf("%s POLL=empty", tag)
			return false, nil
		}
		if bytes.Equal(bs, success) {
			return true, nil
		}
		if bs[0] == 0x04 {
			code := bs[1]
			self.dev.SetErrorCode(int32(code))
			return true, DeviceErrorCode(code)
		}
		return true, self.NewErrPollUnexpected(p)
	}
	return self.dev.NewPollLoop(tag, self.dev.PacketPoll, timeout, fun)
}

func (self *Generic) proto2PollCommon(tag string, bs []byte) (bool, error) {
	if len(bs) == 0 {
		return true, nil
	}
	if len(bs) > 1 {
		return true, errors.Errorf("%s POLL=%x -> too long", tag, bs)
	}
	value := bs[0]
	value &^= self.proto2IgnoreMask
	if bs[0] != 0 && value == 0 {
		self.dev.Log.Debugf("%s proto2-common value=00 bs=%02x ignoring mask=%02x -> success", tag, bs[0], self.proto2IgnoreMask)
		return true, nil
	}
	if value&genericPollProblem != 0 {
		code, err := self.Diagnostic()
		if err != nil {
			err = errors.Annotate(err, tag)
			return true, err
		}
		return true, DeviceErrorCode(code)
	}
	return false, nil
}

func (self *Generic) WithRestart(d engine.Doer) *engine.RestartError {
	return &engine.RestartError{
		Doer: d,
		Check: func(e error) bool {
			_, ok := errors.Cause(e).(DeviceErrorCode)
			return ok
		},
		Reset: engine.Func0{
			Name: d.String() + "/restart-reset",
			F:    self.dev.Reset,
		},
	}
}
