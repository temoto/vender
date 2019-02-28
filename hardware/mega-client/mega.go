package mega

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/brian-armstrong/gpio"
	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/hardware/i2c"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

const modName string = "mega-client"
const DefaultTimeout = 500 * time.Millisecond

type Client struct {
	Log      *log2.Log
	bus      i2c.I2CBus
	addr     byte
	pin      uint
	twich    chan Packet
	refcount int32
	alive    *alive.Alive
	rnd      *rand.Rand
	stat     Stat
}
type Stat struct {
	Error       uint32
	Command     uint32
	ResponseAll uint32
	Twi         uint32
	Stray       uint32
	Reset       uint32
}

func NewClient(busNo byte, addr byte, pin uint, log *log2.Log) (*Client, error) {
	self := &Client{
		Log:   log,
		addr:  addr,
		bus:   i2c.NewI2CBus(busNo),
		pin:   pin,
		alive: alive.NewAlive(),
		twich: make(chan Packet, 16),
		rnd:   helpers.RandUnix(),
	}
	if err := self.bus.Init(); err != nil {
		return nil, err
	}
	go self.pinLoop()
	return self, nil
}

func (self *Client) Close() error {
	self.alive.Stop()
	self.alive.Wait()
	return errors.NotImplementedf("")
}

func (self *Client) IncRef(debug string) {
	self.Log.Debugf("%s incref by %s", modName, debug)
	atomic.AddInt32(&self.refcount, 1)
}
func (self *Client) DecRef(debug string) error {
	self.Log.Debugf("%s decref by %s", modName, debug)
	new := atomic.AddInt32(&self.refcount, -1)
	switch {
	case new > 0:
		return nil
	case new == 0:
		return self.Close()
	}
	panic(fmt.Sprintf("code error %s decref<0 debug=%s", modName, debug))
}

// TODO make it private, used by mega-cli
func (self *Client) RawRead(b []byte) error {
	err := self.bus.Tx(self.addr, nil, b)
	// self.Log.Debugf("%s RawRead addr=%02x buf=%02x err=%v", modName, self.addr, b, err)
	if err != nil {
		atomic.AddUint32(&self.stat.Error, 1)
	}
	return err
}

// TODO make it private, used by mega-cli
func (self *Client) RawWrite(b []byte) error {
	err := self.bus.Tx(self.addr, b, nil)
	if err != nil {
		atomic.AddUint32(&self.stat.Error, 1)
	}
	// self.Log.Debugf("%s RawWrite addr=%02x buf=%02x err=%v", modName, self.addr, b, err)
	return err
}

func (self *Client) DoStatus() (Packet, error) {
	return self.DoTimeout(COMMAND_STATUS, nil, DefaultTimeout)
}

func (self *Client) DoMdbBusReset(d time.Duration) (Packet, error) {
	buf := [2]byte{}
	binary.BigEndian.PutUint16(buf[:], uint16(d/time.Millisecond))
	return self.DoTimeout(COMMAND_MDB_BUS_RESET, buf[:], DefaultTimeout)
}

func (self *Client) DoMdbTxSimple(data []byte) (Packet, error) {
	return self.DoTimeout(COMMAND_MDB_TRANSACTION_SIMPLE, data, DefaultTimeout)
}

func (self *Client) DoTimeout(cmd Command_t, data []byte, timeout time.Duration) (Packet, error) {
	tbegin := time.Now()
	self.Log.Debugf("dotimeout begin")
	defer func() {
		td := time.Now().Sub(tbegin)
		self.Log.Debugf("dotimeout end %v", td)
	}()

	var bufOut [COMMAND_MAX_LENGTH + 1]byte
	var bufIn [RESPONSE_MAX_LENGTH + 1]byte
	var r PacketError

	atomic.AddUint32(&self.stat.Command, 1)
	cmdPacket := NewPacket(
		byte((self.rnd.Uint32()%254)+1),
		byte(cmd),
		data...,
	)
	pb := cmdPacket.Bytes()
	plen := copy(bufOut[:], pb)

	self.ioTx(cmdPacket, bufOut[:plen], bufIn[:], timeout, &r)
	return r.P, r.E
}

func (self *Client) Stat() Stat {
	return self.stat
}

type PacketError struct {
	P Packet
	E error
}

func (self *Client) ioTx(request Packet, bufOut, bufIn []byte, timeout time.Duration, r *PacketError) {
	tbegin := time.Now()

	r.E = self.bus.Tx(self.addr, bufOut, bufIn)
	if r.E != nil {
		return
	}
	for {
		if bufIn[0] > 0 && self.parse(bufIn, request.Id, &r.P) {
			return
		}
		if time.Now().Sub(tbegin) > timeout {
			r.E = errors.Timeoutf("mega/tx")
			return
		}
		time.Sleep(1 * time.Millisecond)
		if r.E = self.RawRead(bufIn); r.E != nil {
			return
		}
	}
}

func (self *Client) pinLoop() {
	stopch := self.alive.StopChan()

	pinWatch := gpio.NewWatcher()
	pinWatch.AddPinWithEdgeAndLogic(self.pin, gpio.EdgeRising, gpio.ActiveHigh)
	defer pinWatch.Close()

	for self.alive.IsRunning() {
		select {
		case <-pinWatch.Notification:
			// self.Log.Debugf("pin edge")
			// self.DoTimeout(COMMAND_STATUS, nil, 10*time.Millisecond)
		case <-stopch:
			return
		}
	}
}

func (self *Client) parse(buf []byte, cmdId byte, dest *Packet) bool {
	match := false
	err := ParseResponse(buf, func(p Packet) {
		// self.Log.Debugf("parsed packet=%02x %s", p.Bytes(), p.String())
		switch Response_t(p.Header) {
		case RESPONSE_JUST_RESET:
			atomic.AddUint32(&self.stat.Reset, 1)
			fields, err := p.ParseFields()
			if err != nil {
				atomic.AddUint32(&self.stat.Error, 1)
				self.Log.Errorf("JUST_RESET ParseFields err=%v", err)
			} else if ResetFlag(fields.Mcusr)&ResetFlagWatchdog != 0 {
				atomic.AddUint32(&self.stat.Error, 1)
				self.Log.Errorf("restarted by watchdog, info=%s", fields.String())
			}
		case RESPONSE_TWI:
			if p.Id != 0 {
				atomic.AddUint32(&self.stat.Error, 1)
				self.Log.Errorf("code error RESPONSE_TWI unexpected Id=%d", p.Id)
				return
			}
			atomic.AddUint32(&self.stat.Twi, 1)
			select {
			case self.twich <- p:
			default:
				self.Log.Errorf("CRITICAL twich chan is full")
			}
		default:
			if p.Id == 0 {
				self.Log.Errorf("non-response=%s", p.String())
			}
		}
		if p.Id != 0 {
			atomic.AddUint32(&self.stat.ResponseAll, 1)
		}
		if p.Id == cmdId {
			*dest = p
			match = true
		} else {
			self.Log.Errorf("CRITICAL (parse) stray packet %s", p.String())
		}
	})
	if err != nil {
		atomic.AddUint32(&self.stat.Error, 1)
		self.Log.Errorf("%s parse=%02x err=%v", modName, buf, err)
	}
	return match
}
