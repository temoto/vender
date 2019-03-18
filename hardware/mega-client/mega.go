package mega

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brian-armstrong/gpio"
	"github.com/juju/errors"
	rpi "github.com/nathan-osman/go-rpigpio"
	"github.com/temoto/alive"
	"github.com/temoto/vender/hardware/i2c"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

const modName string = "mega-client"
const DefaultTimeout = 100 * time.Millisecond

var ErrResponseEmpty = errors.New("mega response empty")

type Client struct {
	Log      *log2.Log
	TwiChan  chan uint16
	txlk     sync.Mutex
	bus      i2c.I2CBus
	addr     byte
	pinready chan struct{}
	refcount int32
	alive    *alive.Alive
	rnd      *rand.Rand
	stat     Stat
}
type Stat struct {
	Error       uint32
	Request     uint32
	ResponseAll uint32
	Reset       uint32
}

func NewClient(busNo byte, addr byte, gpioNo int, log *log2.Log) (*Client, error) {
	self := &Client{
		Log:      log,
		addr:     addr,
		bus:      i2c.NewI2CBus(busNo),
		alive:    alive.NewAlive(),
		pinready: make(chan struct{}),
		TwiChan:  make(chan uint16, TWI_LISTEN_MAX_LENGTH/2),
		rnd:      helpers.RandUnix(),
	}

	if err := self.bus.Init(); err != nil {
		return nil, err
	}
	go self.pinLoop(gpioNo)

	// try to read RESET
	p := new(Packet)
	// TODO check valid outcomes: RESET or empty
	// other packets: log error
	err := self.readParse(p)
	switch err {
	case ErrResponseEmpty:
		// mega was reset at other time and RESET was read
	case nil:
		// TODO is it RESET?
	default:
		self.Close()
		return nil, err
	}

	return self, nil
}

func (self *Client) Close() error {
	close(self.TwiChan)
	close(self.pinready)
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

func (self *Client) DoStatus() (Packet, error) {
	return self.DoTimeout(COMMAND_STATUS, nil, DefaultTimeout)
}

func (self *Client) DoMdbBusReset(d time.Duration) (Packet, error) {
	buf := [2]byte{}
	binary.BigEndian.PutUint16(buf[:], uint16(d/time.Millisecond))
	return self.DoTimeout(COMMAND_MDB_BUS_RESET, buf[:], d+DefaultTimeout)
}

func (self *Client) DoMdbTxSimple(data []byte) (Packet, error) {
	return self.DoTimeout(COMMAND_MDB_TRANSACTION_SIMPLE, data, DefaultTimeout)
}

func (self *Client) DoTimeout(cmd Command_t, data []byte, timeout time.Duration) (Packet, error) {
	// tbegin := time.Now()
	// defer func() {
	// 	td := time.Now().Sub(tbegin)
	// 	self.Log.Debugf("dotimeout end %v", td)
	// }()

	var r PacketError
	var bufSend [REQUEST_MAX_LENGTH + 1]byte

	atomic.AddUint32(&self.stat.Request, 1)
	cmdPacket := NewPacket(
		byte((self.rnd.Uint32()%254)+1),
		byte(cmd),
		data...,
	)
	pb := cmdPacket.Bytes()
	plen := copy(bufSend[:], pb)

	self.Tx(cmdPacket.Id, bufSend[:plen], true, timeout, &r)
	return r.P, r.E
}

func (self *Client) Stat() Stat {
	return self.stat
}

type PacketError struct {
	P Packet
	E error
}

func (self *Client) Tx(requestId byte, bufSend []byte, recv bool, timeout time.Duration, r *PacketError) {
	self.txlk.Lock()
	defer self.txlk.Unlock()

	// self.Log.Debugf("Tx() send=%x", bufSend)
	if bufSend != nil {
		r.E = self.bus.Tx(self.addr, bufSend, nil, 0)
		if r.E != nil {
			atomic.AddUint32(&self.stat.Error, 1)
			return
		}
	}

	// Tx() invoked to send only
	if !recv {
		return
	}

	var deadlineCh <-chan time.Time
	var timerCh <-chan time.Time
	if timeout > 0 {
		deadline := time.NewTimer(timeout)
		defer deadline.Stop()
		deadlineCh = deadline.C
		timer := time.NewTicker(timeout / 4)
		defer timer.Stop()
		timerCh = timer.C
	}
	var timeoutErr error
read:
	r.E = self.readParse(&r.P)
	switch r.E {
	case ErrResponseEmpty:
		// wait again
	case nil:
		if !(requestId == 0 || r.P.Id == requestId) {
			self.Log.Errorf("%s Tx() CRITICAL stray packet=%s", modName, r.P.String())
			r.P = Packet{}
			break // wait again
		}
		return
	default:
		return
	}
	if timeout == 0 {
		return
	}
	if timeoutErr != nil {
		r.E = timeoutErr
		return
	}

	select {
	case <-timerCh:
		self.Log.Debugf("redundant timer")
		goto read
	case <-self.pinready:
		self.Log.Debugf("pin ready")
		goto read
	case <-deadlineCh:
		self.Log.Debugf("deadline")
		timeoutErr = errors.Timeoutf("mega-client Tx() requestId=%02x send=%x response timeout=%s", requestId, bufSend, timeout)
		goto read
	}
}

func (self *Client) pinLoop(gpioNo int) {
	stopch := self.alive.StopChan()
	pin, err := rpi.OpenPin(gpioNo, rpi.IN)
	if err != nil {
		self.Log.Errorf("gpio=%d open err=%v", gpioNo, err)
	}
	pinWatch := gpio.NewWatcher()
	pinWatch.AddPinWithEdgeAndLogic(uint(gpioNo), gpio.EdgeRising, gpio.ActiveHigh)
	defer pinWatch.Close()

	var pinPacket Packet
	for self.alive.IsRunning() {
		select {
		case watchEvent := <-pinWatch.Notification:
			if !(int(watchEvent.Pin) == gpioNo && gpio.Value(watchEvent.Value) == gpio.Active) {
				break
			}
			select {
			case self.pinready <- struct{}{}:
			default:
				// re-read pin with lock to skip reads intended for concurrent Tx()
				self.txlk.Lock()
				pinValue, _ := pin.Read()
				shouldRead := pinValue == rpi.HIGH
				if shouldRead {
					err = self.readParse(&pinPacket)
				}
				self.txlk.Unlock()
				if !shouldRead {
					// self.Log.Errorf("REDUNDANT READ PREVENTED")
					break
				}
				switch err {
				case ErrResponseEmpty:
					self.Log.Errorf("%s pin was high but read empty", modName)
				case nil:
					// TODO rewrite as ".parse() has consumed packet"
					resp := Response_t(pinPacket.Header)
					if resp != RESPONSE_RESET && resp != RESPONSE_TWI_LISTEN {
						self.Log.Errorf("%s pinLoop() CRITICAL stray packet=%s", modName, pinPacket.String())
					}
				default:
					self.Log.Errorf("%s pinLoop() read err=%v", modName, err)
				}
			}
		case <-stopch:
			return
		}
	}
}

func (self *Client) readParse(p *Packet) error {
	var buf [RESPONSE_MAX_LENGTH + 1]byte

	err := self.bus.Tx(self.addr, nil, buf[:], 0)
	self.Log.Debugf("%s read buf=%x err=%v", modName, buf, err)
	if err != nil {
		atomic.AddUint32(&self.stat.Error, 1)
		return err
	}

	// Fix wrong high bit due to Raspberry I2C problems.
	// Luckily, we know for sure that first byte in proper response can't have high bit set.
	if buf[0]&0x80 != 0 {
		buf[0] &^= 0x80
		self.Log.Errorf("CORRUPTED READ")
	}
	if buf[0] == 0 {
		return ErrResponseEmpty
	}

	err = self.parse(buf[:], p)
	// self.Log.Debugf("parse p=%s err=%v", p.String(), err)
	if err != nil {
		atomic.AddUint32(&self.stat.Error, 1)
		return err
	}
	return nil
}

func (self *Client) parse(buf []byte, p *Packet) error {
	err := p.Parse(buf)
	if err != nil {
		atomic.AddUint32(&self.stat.Error, 1)
		self.Log.Errorf("%s parse=%x err=%v", modName, buf, err)
		return err
	}

	// self.Log.Debugf("parsed packet=%x %s", p.Bytes(), p.String())
	if p.Fields.Protocol != ProtocolVersion {
		self.Log.Errorf("Protocol=%d expected=%d", p.Fields.Protocol, ProtocolVersion)
		// return err
	}

	if p.Id == 0 {
		switch Response_t(p.Header) {
		case RESPONSE_TWI_LISTEN:
			for i := 0; i+1 < len(p.Fields.TwiData); i += 2 {
				twitem := binary.BigEndian.Uint16(p.Fields.TwiData[i : i+2])
				select {
				case self.TwiChan <- twitem:
				default:
					self.Log.Errorf("CRITICAL twich chan is full")
				}
			}
		case RESPONSE_RESET:
			atomic.AddUint32(&self.stat.Reset, 1)
			if ResetFlag(p.Fields.Mcusr)&ResetFlagWatchdog != 0 {
				atomic.AddUint32(&self.stat.Error, 1)
				self.Log.Errorf("mega restarted by watchdog, info=%s", p.Fields.String())
			} else {
				self.Log.Debugf("mega normal reset, info=%s", p.Fields.String())
			}
		}
	}

	return nil
}
