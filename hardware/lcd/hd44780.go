package lcd

import (
	"strconv"
	"time"

	"github.com/temoto/gpio-cdev-go"
)

type Command byte

const (
	CommandClear   Command = 0x01
	CommandReturn  Command = 0x02
	CommandControl Command = 0x08
	CommandAddress Command = 0x80
)

type Control byte

const (
	ControlOn         Control = 0x04
	ControlUnderscore Control = 0x02
	ControlBlink      Control = 0x01
)
const ddramWidth = 0x40

type LCD struct {
	control Control
	pinChip *gpio.Chip
	pins    *gpio.LinesHandle
	pin_rs  gpio.LineSetFunc // command/data, aliases: A0, RS
	pin_rw  gpio.LineSetFunc // read/write
	pin_e   gpio.LineSetFunc // enable
	pin_d4  gpio.LineSetFunc
	pin_d5  gpio.LineSetFunc
	pin_d6  gpio.LineSetFunc
	pin_d7  gpio.LineSetFunc
}

type PinMap struct {
	RS string `hcl:"rs"`
	RW string `hcl:"rw"`
	E  string `hcl:"e"`
	D4 string `hcl:"d4"`
	D5 string `hcl:"d5"`
	D6 string `hcl:"d6"`
	D7 string `hcl:"d7"`
}

func (self *LCD) Init(chipName string, pinmap PinMap, page1 bool) error {
	var err error
	self.pinChip, err = gpio.Open(chipName, "lcd")
	if err != nil {
		return err
	}
	nRS := mustAtou32(pinmap.RS)
	nRW := mustAtou32(pinmap.RW)
	nE := mustAtou32(pinmap.E)
	nD4 := mustAtou32(pinmap.D4)
	nD5 := mustAtou32(pinmap.D5)
	nD6 := mustAtou32(pinmap.D6)
	nD7 := mustAtou32(pinmap.D7)
	self.pins, err = self.pinChip.OpenLines(
		gpio.GPIOHANDLE_REQUEST_OUTPUT, "lcd",
		nRS, nRW, nE, nD4, nD5, nD6, nD7,
	)
	if err != nil {
		return err
	}
	self.pin_rs = self.pins.SetFunc(nRS)
	self.pin_rw = self.pins.SetFunc(nRW)
	self.pin_e = self.pins.SetFunc(nE)
	self.pin_d4 = self.pins.SetFunc(nD4)
	self.pin_d5 = self.pins.SetFunc(nD5)
	self.pin_d6 = self.pins.SetFunc(nD6)
	self.pin_d7 = self.pins.SetFunc(nD7)

	self.init4(page1)
	return nil
}

func (self *LCD) setAllPins(b byte) {
	self.pin_rs(b)
	self.pin_rw(b)
	self.pin_e(b)
	self.pin_d4(b)
	self.pin_d5(b)
	self.pin_d6(b)
	self.pin_d7(b)
	self.pins.Flush() //nolint:errcheck
}

func (self *LCD) blinkE() {
	self.pin_e(1)
	// FIXME check error
	self.pins.Flush() //nolint:errcheck
	time.Sleep(1 * time.Microsecond)
	self.pin_e(0)
	// FIXME check error
	self.pins.Flush() //nolint:errcheck
	time.Sleep(1 * time.Microsecond)
}

func (self *LCD) send4(rs, d4, d5, d6, d7 byte) {
	// log.Printf("sn4 %v %v %v %v %v\n", rs, d7, d6, d5, d4)
	self.pin_rs(rs)
	self.pin_d4(d4)
	self.pin_d5(d5)
	self.pin_d6(d6)
	self.pin_d7(d7)
	self.blinkE()
}

func (self *LCD) init4(page1 bool) {
	time.Sleep(20 * time.Millisecond)

	// special sequence
	self.Command(0x33)
	self.Command(0x32)

	self.SetFunction(false, page1)
	self.SetControl(0) // off
	self.SetControl(ControlOn)
	self.Clear()
	self.SetEntryMode(true, false)
}

func bb(b, bit byte) byte {
	if b&(1<<bit) == 0 {
		return 0
	}
	return 1
}

func (self *LCD) Command(c Command) {
	b := byte(c)
	// log.Printf("cmd %0x\n", b)
	self.send4(0, bb(b, 4), bb(b, 5), bb(b, 6), bb(b, 7))
	self.send4(0, bb(b, 0), bb(b, 1), bb(b, 2), bb(b, 3))
	// TODO poll busy flag
	time.Sleep(40 * time.Microsecond)
	self.setAllPins(0)
}

func (self *LCD) Data(b byte) {
	// log.Printf("dat %0x\n", b)
	self.send4(1, bb(b, 4), bb(b, 5), bb(b, 6), bb(b, 7))
	self.send4(1, bb(b, 0), bb(b, 1), bb(b, 2), bb(b, 3))
	// TODO poll busy flag
	time.Sleep(40 * time.Microsecond)
	self.setAllPins(0)
}

func (self *LCD) Write(bs []byte) {
	for _, b := range bs {
		self.Data(b)
	}
}

func (self *LCD) Clear() {
	self.Command(CommandClear)
	// TODO poll busy flag
	time.Sleep(2 * time.Millisecond)
}

func (self *LCD) Return() {
	self.Command(CommandReturn)
}

func (self *LCD) SetEntryMode(right, shift bool) {
	var cmd Command = 0x04
	if right {
		cmd |= 0x02
	}
	if shift {
		cmd |= 0x01
	}
	self.Command(cmd)
}

func (self *LCD) Control() Control {
	return self.control
}
func (self *LCD) SetControl(new Control) Control {
	old := self.control
	self.control = new
	self.Command(CommandControl | Command(new))
	return old
}

func (self *LCD) SetFunction(bits8, page1 bool) {
	var cmd Command = 0x28
	if bits8 {
		cmd |= 0x10
	}
	if page1 {
		cmd |= 0x02
	}
	self.Command(cmd)
}

func (self *LCD) CursorYX(row uint8, column uint8) bool {
	if !(row > 0 && row <= 2) {
		return false
	}
	if !(column > 0 && column <= 16) {
		return false
	}
	addr := (row-1)*ddramWidth + (column - 1)
	self.Command(CommandAddress | Command(addr))
	return true
}

func mustAtou32(s string) uint32 {
	x, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		panic(err)
	}
	return uint32(x)
}
