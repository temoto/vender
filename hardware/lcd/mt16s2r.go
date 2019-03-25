package lcd

import (
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/host"
)

type LCD struct {
	pin_rs  gpio.PinIO // command/data, aliases: A0, RS
	pin_rw  gpio.PinIO // read/write
	pin_e   gpio.PinIO // enable
	pin_db4 gpio.PinIO
	pin_db5 gpio.PinIO
	pin_db6 gpio.PinIO
	pin_db7 gpio.PinIO
}

func openPin(name string, errs *[]error) gpio.PinIO {
	p := gpioreg.ByName(name)
	if p == nil {
		*errs = append(*errs, errors.Errorf("LCD/init unknown pin=%s", name))
		return nil
	}
	if err := p.Out(gpio.Low); err != nil {
		*errs = append(*errs, errors.Annotatef(err, "LCD/init pin.Out() pin=%s", name))
		return nil
	}
	return p
}

func (self *LCD) Init() error {
	if _, err := host.Init(); err != nil {
		return errors.Annotate(err, "periph/init")
	}

	errs := make([]error, 0, 8)
	self.pin_rs = openPin("23", &errs)
	self.pin_rw = openPin("18", &errs)
	self.pin_e = openPin("24", &errs)
	self.pin_db4 = openPin("22", &errs)
	self.pin_db5 = openPin("21", &errs)
	self.pin_db6 = openPin("17", &errs)
	self.pin_db7 = openPin("7", &errs)

	return helpers.FoldErrors(errs)
}

func setPinBool(pin gpio.PinOut, b bool) error {
	level := gpio.Low
	if b {
		level = gpio.High
	}
	return pin.Out(level)
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
	self.pin_e.Out(gpio.High)
	time.Sleep(1 * time.Microsecond)
	self.pin_e.Out(gpio.Low)
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
