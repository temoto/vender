package mega

import (
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brian-armstrong/gpio"
	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/hardware/i2c"
	"github.com/temoto/vender/helpers"
)

const modName string = "mega-client"
const DefaultTimeout = 500 * time.Millisecond

type Client struct {
	bus       i2c.I2CBus
	addr      byte
	pin       uint
	twiCh     chan Packet
	respCh    chan Packet
	strayCh   chan Packet
	refcount  int32
	alive     *alive.Alive
	serialize sync.Mutex
	rnd       *rand.Rand
	expectId  uint32
}

func NewClient(busNo byte, addr byte, pin uint) (*Client, error) {
	self := &Client{
		addr:    addr,
		bus:     i2c.NewI2CBus(busNo),
		pin:     pin,
		alive:   alive.NewAlive(),
		respCh:  make(chan Packet, 16),
		strayCh: make(chan Packet, 16),
		twiCh:   make(chan Packet, 16),
		rnd:     helpers.RandUnix(),
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
	log.Printf("%s incref by %s", modName, debug)
	atomic.AddInt32(&self.refcount, 1)
}
func (self *Client) DecRef(debug string) error {
	log.Printf("%s decref by %s", modName, debug)
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
		// log.Printf("%s RawRead addr=%02x error=%v", modName, self.addr, err)
		return err
	}
	// log.Printf("%s RawRead addr=%02x buf=%02x", modName, self.addr, b)
	return nil
}

// TODO make it private, used by mega-cli
func (self *Client) RawWrite(b []byte) error {
	err := self.bus.WriteBytesAt(self.addr, b)
	// log.Printf("%s RawWrite addr=%02x buf=%02x err=%v", modName, self.addr, b, err)
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

	bufOut := make([]byte, COMMAND_MAX_LENGTH)
	cmdPacket := Packet{
		Id:     byte((self.rnd.Uint32() % 254) + 1),
		Header: byte(cmd),
		Data:   data,
	}

	// FIXME n,_:=p.WriteTo(bufOut)
	pb := cmdPacket.Bytes()
	plen := copy(bufOut, pb)
	err := self.RawWrite(bufOut[:plen])
	if err != nil {
		return Packet{}, err
	}

	for try := 1; try < 5; try++ {
		select {
		case resPacket := <-self.respCh:
			if resPacket.Id != 0 && resPacket.Id != cmdPacket.Id {
				log.Printf("CRITICAL stray command=%02x response=%s", cmdPacket.Bytes(), resPacket.String())
				// self.strayCh <- resPacket
				break
			}
			return resPacket, nil
		case <-time.After(timeout):
			err = errors.Timeoutf("omg") // FIXME text
			return Packet{}, err
		}
	}
	err = errors.Errorf("too many unexpected response packets")
	return Packet{}, err
}

func (self *Client) reader() {
	stopch := self.alive.StopChan()
	bufIn := make([]byte, RESPONSE_MAX_LENGTH)

	pinWatch := gpio.NewWatcher()
	pinWatch.AddPinWithEdgeAndLogic(self.pin, gpio.EdgeRising, gpio.ActiveHigh)
	defer pinWatch.Close()

	for self.alive.IsRunning() {
		select {
		case <-pinWatch.Notification:
			err := self.RawRead(bufIn)
			if err != nil {
				log.Printf("%s pin read=%02x error=%v", modName, self.addr, err)
				break
			}
			err = ParseResponse(bufIn, func(p Packet) {
				log.Printf("debug pin parsed packet=%02x %s", p.Bytes(), p.String())
				switch Response_t(p.Header) {
				case RESPONSE_TWI:
					self.twiCh <- p
				default:
					if p.Id == 0 {
						log.Printf("INFO non-response=%s", p.String())
						// self.strayCh <- p
					} else {
						self.respCh <- p
					}
				}
			})
			if err != nil {
				log.Printf("pin read=%02x parse error=%v", bufIn, err)
				break
			}

		case <-stopch:
			return
		}
	}
}
