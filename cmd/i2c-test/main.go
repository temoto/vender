package main

import (
	"fmt"
	"log"
	"time"

	"github.com/temoto/vender/crc"
	"github.com/temoto/vender/hardware/i2c"
)

func main1() {
	addr := byte(0x70)
	twi := i2c.NewI2CBus(0)

	//buf := []byte{0x80, 0x00, 0x74}
	//err := twi.WriteBytes(addr, buf)
	//log.Printf("write err=%v", err)
	//time.Sleep(10 * time.Millisecond)
	//for i := 0; i < len(buf); i++ {
	//  	buf[i] = 0
	//}
	//log.Printf("read err=%v buf=%02x%02x%02x", err, buf[0], buf[1], buf[2])

	for {
		b, err := twi.ReadByteAt(addr)
		if err == nil {
			log.Printf("read %02x", b)
		} else {
			log.Printf("read err=%v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	// twi.
}

type megaMsg [3]byte

type tmega struct {
	bus            i2c.I2CBus
	Addr           byte
	ReadRetryDelay time.Duration
}

func newMegaMsg(cmd, data byte) (out megaMsg) {
	out[0] = cmd
	out[1] = data
	out[2] = crc.CRC8_p93_2(cmd, data)
	return out
}

func (m megaMsg) IsTryAgain() bool {
	return m[0] == 0x01 && m[1] == 0 && m[2] == 0
}

func (m megaMsg) String() string {
	return fmt.Sprintf("%02x%02x%02x", m[0], m[1], m[2])
}

func (self *tmega) SendUART(data byte, bit9 bool) error {
	cmd := byte(0xa0)
	if bit9 {
		cmd = 0xb0
	}
	msg := newMegaMsg(cmd, data)
	log.Printf("SendUART msg %s", msg.String())
	return self.bus.WriteBytesAt(self.Addr, msg[:])
}

func (self *tmega) ReadMsg(timeout time.Duration) (out megaMsg, err error) {
	retryEnd := time.Now().Add(timeout)
	for {
		_, err = self.bus.ReadBytesAt(self.Addr, out[:])
		if err != nil || !out.IsTryAgain() || time.Now().After(retryEnd) {
			break
		}
		time.Sleep(self.ReadRetryDelay)
	}
	return
}

func main() {
	mega := tmega{
		bus:            i2c.NewI2CBus(0),
		Addr:           0x78,
		ReadRetryDelay: 10 * time.Millisecond,
	}
	var err error
	var msg megaMsg
	time.Sleep(mega.ReadRetryDelay)
	if err = mega.SendUART(0x30, true); err != nil {
		log.Printf("SendUART error %v", err)
	}
	time.Sleep(mega.ReadRetryDelay)
	if msg, err = mega.ReadMsg(1 * time.Second); err != nil {
		log.Printf("Read error %v", err)
	}
	log.Printf("ReadMsg %s", msg.String())
	time.Sleep(mega.ReadRetryDelay)
	if err = mega.SendUART(0x30, false); err != nil {
		log.Printf("SendUART error %v", err)
	}
	time.Sleep(mega.ReadRetryDelay)
	if msg, err = mega.ReadMsg(1 * time.Second); err != nil {
		log.Printf("Read error %v", err)
	}
	log.Printf("ReadMsg %s", msg.String())
}
