package mega

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/crc"
	"github.com/temoto/vender/hardware/i2c"
)

const modName string = "mega-client"

type Client struct {
	bus i2c.I2CBus
	// TODO listen gpio pin
	addr     byte
	refcount int32
	alive    *alive.Alive
	out      chan Tx
	in       chan []byte
}

func NewClient(busNo byte, addr byte) (*Client, error) {
	c := &Client{
		addr:  addr,
		bus:   i2c.NewI2CBus(busNo),
		alive: alive.NewAlive(),
		out:   make(chan Tx),
		in:    make(chan []byte),
	}
	go c.run()
	return c, nil
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
	n, err := self.bus.ReadBytesAt(self.addr, b)
	if err != nil {
		log.Printf("%s RawRead addr=%02x error=%v", modName, self.addr, err)
		return err
	}
	log.Printf("%s RawRead addr=%02x n=%v buf=%02x", modName, self.addr, n, b)
	return nil
}

// TODO make it private, used by mega-cli
func (self *Client) RawWrite(b []byte) error {
	err := self.bus.WriteBytesAt(self.addr, b)
	log.Printf("%s RawWrite addr=%02x buf=%02x err=%v", modName, self.addr, b, err)
	return err
}

type Tx struct {
	Rq []byte
	Rs []byte
	Ps []Packet
	E  error
}

func (self *Client) Do(t *Tx) error {
	plen := len(t.Rq) + 2
	packet := make([]byte, plen)
	packet[0] = byte(plen)
	copy(packet[1:], t.Rq)
	packet[plen-1] = crc.CRC8_p93_n(0, packet[:plen-1])
	log.Printf("%s Do packet=%02x", modName, packet)

	t.E = self.RawWrite(packet)
	if t.E != nil {
		return t.E
	}
	// FIXME wait pin edge
	time.Sleep(100 * time.Millisecond)
	t.Rs = make([]byte, RESPONSE_MAX_LENGTH+1)
	t.E = self.RawRead(t.Rs)
	if t.E != nil {
		return t.E
	}

	guessPacketCount := 0
	if len(t.Rs) > 0 {
		guessPacketCount = int(t.Rs[0]) / 5
	}
	t.Ps = make([]Packet, 0, guessPacketCount)
	t.E = ParseResponse(t.Rs, func(p Packet) {
		t.Ps = append(t.Ps, p)
	})
	return t.E
}

func (self *Client) run() {
	stopch := self.alive.StopChan()
	// TODO listen gpio pin
	backup := time.NewTicker(740 * time.Millisecond)
	backup.Stop()
	buf := make([]byte, RESPONSE_MAX_LENGTH+1)
	for self.alive.IsRunning() {
		select {
		case tx := <-self.out:
			_ = self.RawWrite(tx.Rq)
			// TODO listen gpio pin
		case <-backup.C:
			// TODO expect empty, otherwise log "gpio fail"
			err := self.RawRead(buf)
			if err != nil {
				log.Printf("%s backup read=%02x error=%v", modName, self.addr, err)
				break
			}
			log.Printf("%s backup read buf=%02x", modName, buf)
		case <-stopch:
			return
		}
	}
}
