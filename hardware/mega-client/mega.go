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
	"github.com/temoto/alive"
	"github.com/temoto/vender/hardware/i2c"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/helpers/msync"
	"github.com/temoto/vender/log2"
)

const modName string = "mega-client"
const DefaultTimeout = 500 * time.Millisecond

type Client struct {
	Log        *log2.Log
	bus        i2c.I2CBus
	addr       byte
	pin        uint
	respCh     chan Packet
	strayCh    chan Packet
	twiCh      chan Packet
	readSignal msync.Signal
	refcount   int32
	alive      *alive.Alive
	serialize  sync.Mutex
	rnd        *rand.Rand
	stat       Stat
}
type Stat struct {
	Error         uint
	Command       uint
	ResponseAll   uint
	ResponseMatch uint
	Twi           uint
	Stray         uint
}

func NewClient(busNo byte, addr byte, pin uint, log *log2.Log) (*Client, error) {
	self := &Client{
		Log:        log,
		addr:       addr,
		bus:        i2c.NewI2CBus(busNo),
		pin:        pin,
		alive:      alive.NewAlive(),
		respCh:     make(chan Packet, 16),
		strayCh:    make(chan Packet, 16),
		twiCh:      make(chan Packet, 16),
		readSignal: msync.NewSignal(),
		rnd:        helpers.RandUnix(),
	}
	if err := self.bus.Init(); err != nil {
		return nil, err
	}
	go self.reader()
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
	_, err := self.bus.ReadBytesAt(self.addr, b)
	if err != nil {
		self.stat.Error++
		// self.Log.Debugf("%s RawRead addr=%02x error=%v", modName, self.addr, err)
		return err
	}
	// self.Log.Debugf("%s RawRead addr=%02x buf=%02x", modName, self.addr, b)
	return nil
}

// TODO make it private, used by mega-cli
func (self *Client) RawWrite(b []byte) error {
	err := self.bus.WriteBytesAt(self.addr, b)
	if err != nil {
		self.stat.Error++
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
	self.serialize.Lock()
	defer self.serialize.Unlock()

	self.stat.Command++

	bufOut := make([]byte, COMMAND_MAX_LENGTH)
	cmdPacket := NewPacket(
		byte((self.rnd.Uint32()%254)+1),
		byte(cmd),
		data...,
	)

	// FIXME n,_:=p.WriteTo(bufOut)
	pb := cmdPacket.Bytes()
	plen := copy(bufOut, pb)
	err := self.RawWrite(bufOut[:plen])
	if err != nil {
		self.stat.Error++
		return Packet{}, err
	}

	for try := 1; try < 5; try++ {
		select {
		case resPacket := <-self.respCh:
			if resPacket.Id != 0 {
				if resPacket.Id != cmdPacket.Id {
					self.stat.Stray++
					self.Log.Errorf("CRITICAL stray command=%02x response=%s", cmdPacket.Bytes(), resPacket.String())
					// self.strayCh <- resPacket
					break
				}
				self.stat.ResponseMatch++
			}
			return resPacket, nil
		case <-time.After(timeout):
			self.stat.Error++
			self.readSignal.Set()
			err = errors.Timeoutf("omg") // FIXME text
			return Packet{}, err
		}
	}
	err = errors.Errorf("too many unexpected response packets")
	return Packet{}, err
}

func (self *Client) Stat() Stat {
	return self.stat
}

func (self *Client) reader() {
	bufIn := make([]byte, RESPONSE_MAX_LENGTH)
	stopch := self.alive.StopChan()

	pinWatch := gpio.NewWatcher()
	pinWatch.AddPinWithEdgeAndLogic(self.pin, gpio.EdgeRising, gpio.ActiveHigh)
	defer pinWatch.Close()

	for self.alive.IsRunning() {
		select {
		case <-pinWatch.Notification:
			self.read(bufIn)
		case <-self.readSignal:
			self.read(bufIn)
		case <-stopch:
			return
		}
	}
}

func (self *Client) read(buf []byte) {
	err := self.RawRead(buf)
	if err != nil {
		self.stat.Error++
		self.Log.Errorf("%s pin read=%02x error=%v", modName, self.addr, err)
		return
	}
	err = ParseResponse(buf, func(p Packet) {
		// self.Log.Debugf("pin parsed packet=%02x %s", p.Bytes(), p.String())
		switch Response_t(p.Header) {
		case RESPONSE_TWI:
			self.stat.Twi++
			self.twiCh <- p
		default:
			if p.Id == 0 {
				self.Log.Infof("non-response=%s", p.String())
				// self.strayCh <- p
			} else {
				self.stat.ResponseAll++
				self.respCh <- p
			}
		}
	})
	if err != nil {
		self.stat.Error++
		self.Log.Errorf("pin read=%02x parse error=%v", buf, err)
	}
}
