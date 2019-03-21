package lcd

import (
	"time"

	rpi "github.com/nathan-osman/go-rpigpio"
)

type LCD struct {
	pin_rs  *rpi.Pin // command/data, aliases: A0, RS
	pin_rw  *rpi.Pin // read/write
	pin_e   *rpi.Pin // enable
	pin_db4 *rpi.Pin
	pin_db5 *rpi.Pin
	pin_db6 *rpi.Pin
	pin_db7 *rpi.Pin
}

func (self *LCD) Init() (err error) {
	self.pin_rs, err = rpi.OpenPin(23, rpi.OUT)
	if err != nil {
		return
	}
	self.pin_rw, err = rpi.OpenPin(18, rpi.OUT)
	if err != nil {
		return
	}
	self.pin_e, err = rpi.OpenPin(24, rpi.OUT)
	if err != nil {
		return
	}
	self.pin_db4, err = rpi.OpenPin(22, rpi.OUT)
	if err != nil {
		return
	}
	self.pin_db5, err = rpi.OpenPin(21, rpi.OUT)
	if err != nil {
		return
	}
	self.pin_db6, err = rpi.OpenPin(17, rpi.OUT)
	if err != nil {
		return
	}
	self.pin_db7, err = rpi.OpenPin(7, rpi.OUT)
	if err != nil {
		return
	}
	self.setAllPins(false)

	return nil
}

func setPinBool(pin *rpi.Pin, value bool) error {
	var v rpi.Value = rpi.LOW
	if value {
		v = rpi.HIGH
	}
	return pin.Write(v)
}

func (self *LCD) setAllPins(b bool) {
	setPinBool(self.pin_rs, b)
	setPinBool(self.pin_rw, b)
	setPinBool(self.pin_e, b)
	setPinBool(self.pin_db4, b)
	setPinBool(self.pin_db5, b)
	setPinBool(self.pin_db6, b)
	setPinBool(self.pin_db7, b)
}

func (self *LCD) blinkE() {
	setPinBool(self.pin_e, true)
	time.Sleep(1 * time.Microsecond)
	setPinBool(self.pin_e, false)
	time.Sleep(1 * time.Microsecond)
}

func (self *LCD) send4(rs, db4, db5, db6, db7 bool) {
	// log.Printf("sn4 %v %v %v %v %v\n", rs, db7, db6, db5, db4)
	setPinBool(self.pin_rs, rs)
	setPinBool(self.pin_db4, db4)
	setPinBool(self.pin_db5, db5)
	setPinBool(self.pin_db6, db6)
	setPinBool(self.pin_db7, db7)
	self.blinkE()
}

func (self *LCD) Init4() {
	time.Sleep(20 * time.Millisecond)

	// special sequence
	self.Command(0x33)
	self.Command(0x32)

	self.CommandFunction(false, true)
	self.CommandOff()
	self.CommandOn(true, false, false)
	self.CommandClear()
	self.CommandEntryMode(true, false)
}

func (self *LCD) Command(b byte) {
	// log.Printf("cmd %0x\n", b)
	self.send4(false, b&(1<<4) != 0, b&(1<<5) != 0, b&(1<<6) != 0, b&(1<<7) != 0)
	self.send4(false, b&(1<<0) != 0, b&(1<<1) != 0, b&(1<<2) != 0, b&(1<<3) != 0)
	// TODO poll busy flag
	time.Sleep(40 * time.Microsecond)
	self.setAllPins(false)
}

func (self *LCD) Data(b byte) {
	// log.Printf("dat %0x\n", b)
	self.send4(true, b&(1<<4) != 0, b&(1<<5) != 0, b&(1<<6) != 0, b&(1<<7) != 0)
	self.send4(true, b&(1<<0) != 0, b&(1<<1) != 0, b&(1<<2) != 0, b&(1<<3) != 0)
	// TODO poll busy flag
	time.Sleep(40 * time.Microsecond)
	self.setAllPins(false)
}

func (self *LCD) WriteBytes(bs []byte) {
	for _, b := range bs {
		switch b {
		case '\n':
			self.CommandAddress(0x40)
		default:
			self.Data(b)
		}
	}
}

func (self *LCD) CommandClear() {
	self.Command(0x01)
	// TODO poll busy flag
	time.Sleep(2 * time.Millisecond)
}

func (self *LCD) CommandReturn() {
	self.Command(0x02)
}

func (self *LCD) CommandEntryMode(right, shift bool) {
	var cmd byte = 0x04
	if right {
		cmd |= 0x02
	}
	if shift {
		cmd |= 0x01
	}
	self.Command(cmd)
}

func (self *LCD) CommandOn(on, underscore, blink bool) {
	var cmd byte = 0x08
	if on {
		cmd |= 0x04
	}
	if underscore {
		cmd |= 0x02
	}
	if blink {
		cmd |= 0x01
	}
	self.Command(cmd)
}

func (self *LCD) CommandOff() {
	self.Command(0x08)
}

func (self *LCD) CommandFunction(bits8, page1 bool) {
	var cmd byte = 0x28
	if bits8 {
		cmd |= 0x10
	}
	if page1 {
		cmd |= 0x02
	}
	self.Command(cmd)
}

func (self *LCD) CommandAddress(a byte) {
	self.Command(0x80 | a)
}
