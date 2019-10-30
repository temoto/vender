package mdb

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/helpers/atomic_clock"
	"github.com/temoto/vender/log2"
)

const ErrCodeNone int32 = -1

const (
	DefaultDelayAfterReset  = 500 * time.Millisecond
	DefaultDelayBeforeReset = 0
	DefaultDelayIdle        = 700 * time.Millisecond
	DefaultDelayNext        = 200 * time.Millisecond
	DefaultDelayOffline     = 10 * time.Second
	DefaultIdleThreshold    = 30 * time.Second
)

type Device struct { //nolint:maligned
	state   uint32 // atomic
	errCode int32  // atomic

	bus   *Bus
	cmdLk sync.Mutex // TODO explore if chan approach is better

	LastOk      *atomic_clock.Clock // last successful tx(), 0 at init, monotonic
	LastOff     *atomic_clock.Clock // last change from online to offline (MDB timeout), 0=online
	lastReset   *atomic_clock.Clock // last RESET attempt, 0 only at init, monotonic
	Log         *log2.Log
	Address     uint8
	Name        string
	ByteOrder   binary.ByteOrder
	PacketReset Packet
	PacketSetup Packet
	PacketPoll  Packet
	DoReset     engine.Doer
	DoInit      engine.Doer // likely Seq starting with DoReset

	DelayAfterReset  time.Duration
	DelayBeforeReset time.Duration
	DelayIdle        time.Duration
	DelayNext        time.Duration
	DelayOffline     time.Duration
	IdleThreshold    time.Duration

	SetupResponse Packet
}

func (self *Device) Init(bus *Bus, addr uint8, name string, byteOrder binary.ByteOrder) {
	self.cmdLk.Lock()
	defer self.cmdLk.Unlock()

	self.Address = addr
	self.ByteOrder = byteOrder
	self.Log = bus.Log
	self.bus = bus
	self.Name = name
	self.errCode = ErrCodeNone
	self.LastOk = atomic_clock.New(0)
	self.LastOff = atomic_clock.Now()
	self.lastReset = atomic_clock.New(0)

	if self.DelayAfterReset == 0 {
		self.DelayAfterReset = DefaultDelayAfterReset
	}
	if self.DelayBeforeReset == 0 {
		self.DelayBeforeReset = DefaultDelayBeforeReset
	}
	if self.DelayIdle == 0 {
		self.DelayIdle = DefaultDelayIdle
	}
	if self.DelayNext == 0 {
		self.DelayNext = DefaultDelayNext
	}
	if self.DelayOffline == 0 {
		self.DelayOffline = DefaultDelayOffline
	}
	if self.IdleThreshold == 0 {
		self.IdleThreshold = DefaultIdleThreshold
	}
	self.SetupResponse = Packet{}
	self.PacketReset = MustPacketFromBytes([]byte{self.Address + 0}, true)
	self.PacketSetup = MustPacketFromBytes([]byte{self.Address + 1}, true)
	self.PacketPoll = MustPacketFromBytes([]byte{self.Address + 3}, true)
	self.DoReset = engine.Func0{Name: fmt.Sprintf("mdb.%s.reset", self.Name), F: self.Reset}
	self.SetState(DeviceInited)

	if _, ok := bus.u.(*MockUart); ok {
		// testing
		self.XXX_FIXME_SetAllDelays(1)
	}
}

func (self *Device) TeleError(e error) { self.bus.Error(e) }

func (self *Device) ValidateErrorCode() error {
	value := atomic.LoadInt32(&self.errCode)
	if value == ErrCodeNone {
		return nil
	}
	return errors.Errorf("mdb.%s unhandled errorcode=%d", self.Name, value)
}

func (self *Device) ValidateOnline() error {
	st := self.State()
	if st.Online() {
		return nil
	}
	return errors.Errorf("mdb.%s state=%s offline duration=%v", self.Name, st.String(), atomic_clock.Since(self.LastOff))
}

// Command is known to be supported, MDB timeout means remote is offline.
// RESET if appropriate.
func (self *Device) TxKnown(request Packet, response *Packet) error {
	self.cmdLk.Lock()
	defer self.cmdLk.Unlock()
	return self.txKnown(request, response)
}

// Remote may ignore command with MDB timeout.
// state=Offline -> RESET
// state.Ok() required
func (self *Device) TxMaybe(request Packet, response *Packet) error {
	self.cmdLk.Lock()
	defer self.cmdLk.Unlock()
	st := self.State()
	err := self.tx(request, response, txOptMaybe)
	return errors.Annotatef(err, "mdb.%s TxMaybe request=%x state=%s", self.Name, request.Bytes(), st.String())
}

func (self *Device) TxCustom(request Packet, response *Packet, opt TxOpt) error {
	self.cmdLk.Lock()
	defer self.cmdLk.Unlock()
	return self.tx(request, response, opt)
}

func (self *Device) TxSetup() error {
	err := self.TxKnown(self.PacketSetup, &self.SetupResponse)
	return errors.Annotatef(err, "mdb.%s SETUP", self.Name)
}

func (self *Device) ErrorCode() int32 { return atomic.LoadInt32(&self.errCode) }
func (self *Device) SetErrorCode(c int32) {
	prev := atomic.SwapInt32(&self.errCode, c)
	if prev != ErrCodeNone && c != ErrCodeNone {
		self.Log.Errorf("mdb.%s CRITICAL SetErrorCode overwrite previous=%d", self.Name, prev)
		// TODO tele
	}
	if prev == ErrCodeNone && c != ErrCodeNone {
		self.SetState(DeviceError)
		err := fmt.Errorf("mdb.%s errcode=%d", self.Name, c)
		self.TeleError(err)
	}
}

func (self *Device) State() DeviceState       { return DeviceState(atomic.LoadUint32(&self.state)) }
func (self *Device) Ready() bool              { return self.State() == DeviceReady }
func (self *Device) SetState(new DeviceState) { atomic.StoreUint32(&self.state, uint32(new)) }
func (self *Device) SetReady()                { self.SetState(DeviceReady) }
func (self *Device) SetOnline()               { self.SetState(DeviceOnline) }

func (self *Device) Reset() error {
	self.cmdLk.Lock()
	defer self.cmdLk.Unlock()
	return self.locked_reset()
}

// Keep particular devices "hot" to reduce useless POLL time.
func (self *Device) Keepalive(interval time.Duration, stopch <-chan struct{}) {
	wait := interval

	for {
		// TODO try and benchmark time.After vs NewTimer vs NewTicker
		// self.Log.Debugf("keepalive wait=%v", wait)
		if wait <= 0 {
			wait = 1
		}
		select {
		case <-stopch:
			return
		case <-time.After(wait):
		}
		self.cmdLk.Lock()
		// // state could be updated during Lock()
		// if self.State().Ok() {
		okAge := atomic_clock.Since(self.LastOk)
		wait = interval - okAge
		// self.Log.Debugf("keepalive locked okage=%v wait=%v", okAge, wait)
		if wait <= 0 {
			self.txKnown(self.PacketPoll, new(Packet))
			wait = interval
		}
		self.cmdLk.Unlock()
	}
}

type PollFunc func(Packet) (stop bool, err error)

// Send `request` packets until `timeout` or `fun` returns stop=true or error.
func (self *Device) NewPollLoop(tag string, request Packet, timeout time.Duration, fun PollFunc) engine.Doer {
	tag += "/poll-loop"
	return engine.Func{Name: tag, F: func(ctx context.Context) error {
		tbegin := time.Now()

		self.cmdLk.Lock()
		defer self.cmdLk.Unlock()
		for {
			response := Packet{}
			if err := self.txKnown(request, &response); err != nil {
				return errors.Annotate(err, tag)
			}
			stop, err := fun(response)
			if err == nil && stop { // success
				return nil
			} else if err == nil && !stop { // try again
				if timeout == 0 {
					return errors.Errorf("tag=%s timeout=0 invalid", tag)
				}
				time.Sleep(self.DelayNext)
				if time.Since(tbegin) > timeout {
					return errors.Timeoutf(tag)
				}
				continue
			}

			return errors.Annotate(err, tag)
		}
	}}
}

// Used by tests to avoid waiting.
func (self *Device) XXX_FIXME_SetAllDelays(d time.Duration) {
	self.DelayIdle = d
	self.DelayNext = d
	self.DelayBeforeReset = d
	self.DelayAfterReset = d
	self.DelayOffline = d
}

// cmdLk used to ensure no concurrent commands during delays
func (self *Device) locked_reset() error {
	tag := fmt.Sprintf("mdb.%s", self.Name)

	resetAge := atomic_clock.Since(self.lastReset)
	if resetAge < self.DelayOffline { // don't RESET too often
		self.Log.Debugf("%s locked_reset delay=%v", tag, self.DelayOffline-resetAge)
		time.Sleep(self.DelayOffline - resetAge)
	}

	// st := self.State()
	// self.Log.Debugf("%s locked_reset begin state=%s", tag, st.String())
	self.LastOff.SetNowIfZero() // consider device offline from now till successful response
	// self.SetState(DeviceInited)
	time.Sleep(self.DelayBeforeReset)
	err := self.tx(self.PacketReset, new(Packet), txOptReset)
	// self.Log.Debugf("%s locked_reset after state=%s r.E=%v r.P=%s", tag, st.String(), r.E, r.P.Format())
	self.lastReset.SetNow()
	atomic.StoreInt32(&self.errCode, ErrCodeNone)
	if err != nil {
		if !IsResponseTimeout(err) {
			// TODO remove log here when ensured that error is logged in all callers
			// - Keepalive() ignores err
			// - Reset() returns err to caller without logging
			self.Log.Errorf("%s RESET err=%s", tag, errors.ErrorStack(err))
		}
		err = errors.Annotatef(err, "%s RESET", tag)
		return err
	}
	self.Log.Infof("%s addr=%02x is working", tag, self.Address)
	time.Sleep(self.DelayAfterReset)
	return nil
}

func (self *Device) txKnown(request Packet, response *Packet) error {
	st := self.State()
	self.Log.Debugf("mdb.%s txKnown request=%x state=%s", self.Name, request.Bytes(), st.String())
	return self.tx(request, response, txOptKnown)
}

func (self *Device) tx(request Packet, response *Packet, opt TxOpt) error {
	var err error
	st := self.State()
	switch st {
	case DeviceInvalid:
		return errors.Annotatef(ErrStateInvalid, "mdb.%s", self.Name)

	case DeviceInited: // success path
		if !opt.NoReset {
			err = self.locked_reset()
		}

	case DeviceOnline, DeviceReady: // success path

	case DeviceError: // FIXME TODO remove DeviceError state
		if opt.ResetError && !opt.NoReset {
			err = self.locked_reset()
		}

	case DeviceOffline:
		self.Log.Debugf("mdb.%s tx request=%x state=%s offline duration=%v", self.Name, request.Bytes(), st.String(), atomic_clock.Since(self.LastOff))
		if opt.ResetOffline && !opt.NoReset {
			err = self.locked_reset()
		}

	default:
		panic(fmt.Sprintf("code error mdb.%s tx request=%x unknown state=%v", self.Name, request.Bytes(), st))
	}
	if opt.RequireOK {
		if st2 := self.State(); !st2.Ok() {
			err = ErrStateInvalid
		}
	}

	err = self.bus.Tx(request, response)
	if err == nil {
		// self.Log.Debugf("mdb.%s since last ok %v", self.Name, atomic_clock.Since(self.LastOk))
		self.LastOk.SetNow()
		self.LastOff.Set(0)
		// Upgrade any state except Ready to Online
		// Ready->Online would loose calibration.
		if st != DeviceReady {
			self.SetState(DeviceOnline)
		}
		atomic.StoreInt32(&self.errCode, ErrCodeNone)
	} else if IsResponseTimeout(err) {
		if opt.TimeoutOffline {
			self.LastOff.SetNowIfZero()
			self.SetState(DeviceOffline)
			err = errors.Wrapf(err, ErrOffline, "mdb.%s is offline", self.Name)
			if st != DeviceOffline {
				self.TeleError(err)
			}
		}
		// } else { // other error
	}
	self.Log.Debugf("mdb.%s tx request=%x -> ok=%t timeout=%t state %s -> %s err=%v",
		self.Name, request.Bytes(), err == nil, IsResponseTimeout(err), st.String(), self.State().String(), err)
	return err
}

// "Idle mode" polling, runs forever until receive on `stopch`.
// Switches between fast/idle delays.
// Used by bill/coin devices.
type PollDelay struct {
	lastActive time.Time
	lastDelay  time.Duration
}

func (self *PollDelay) Delay(dev *Device, active bool, err bool, stopch <-chan struct{}) bool {
	delay := dev.DelayNext

	if err {
		delay = dev.DelayIdle
	} else if active {
		self.lastActive = time.Now()
	} else if self.lastDelay != dev.DelayIdle { // save time syscall while idle continues
		if time.Since(self.lastActive) > dev.IdleThreshold {
			delay = dev.DelayIdle
		}
	}
	self.lastDelay = delay

	select {
	case <-stopch:
		return false
	case <-time.After(delay):
		return true
	}
}

type TxOpt struct {
	TimeoutOffline bool
	RequireOK      bool
	NoReset        bool
	ResetError     bool
	ResetOffline   bool
}

var (
	txOptKnown = TxOpt{
		TimeoutOffline: true,
		ResetOffline:   true,
		ResetError:     true,
	}
	txOptMaybe = TxOpt{
		RequireOK:    true,
		ResetOffline: true,
	}
	txOptReset = TxOpt{
		TimeoutOffline: true,
		NoReset:        true,
	}
)
