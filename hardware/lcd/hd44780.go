package lcd

import (
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/host"
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
	pin_rs  gpio.PinIO // command/data, aliases: A0, RS
	pin_rw  gpio.PinIO // read/write
	pin_e   gpio.PinIO // enable
	pin_db4 gpio.PinIO
	pin_db5 gpio.PinIO
	pin_db6 gpio.PinIO
	pin_db7 gpio.PinIO
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

func openPin(tag, name string, errs *[]error) gpio.PinIO {
	if name == "" {
		*errs = append(*errs, errors.Errorf("LCD/init %s pin is not configured", tag))
		return nil
	}
	p := gpioreg.ByName(name)
	if p == nil {
		*errs = append(*errs, errors.Errorf("LCD/init %s pin=%s invalid", tag, name))
		return nil
	}
	if err := p.Out(gpio.Low); err != nil {
		*errs = append(*errs, errors.Annotatef(err, "LCD/init pin.Out() %s pin=%s", tag, name))
		return nil
	}
	return p
}

func (self *LCD) Init(pinmap PinMap) error {
	if _, err := host.Init(); err != nil {
		return errors.Annotate(err, "periph/init")
	}

	errs := make([]error, 0, 8)
	self.pin_rs = openPin("RS/A0", pinmap.RS, &errs)
	self.pin_rw = openPin("RW", pinmap.RW, &errs)
	self.pin_e = openPin("E", pinmap.E, &errs)
	self.pin_db4 = openPin("D4", pinmap.D4, &errs)
	self.pin_db5 = openPin("D5", pinmap.D5, &errs)
	self.pin_db6 = openPin("D6", pinmap.D6, &errs)
	self.pin_db7 = openPin("D7", pinmap.D7, &errs)

	if err := helpers.FoldErrors(errs); err != nil {
		return err
	}

	self.init4()
	return nil
}

func setPinBool(pin gpio.PinOut, b bool) {
	level := gpio.Low
	if b {
		level = gpio.High
	}
	// FIXME check error
	pin.Out(level) //nolint:errcheck
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
	// FIXME check error
	self.pin_e.Out(gpio.High) //nolint:errcheck
	time.Sleep(1 * time.Microsecond)
	// FIXME check error
	self.pin_e.Out(gpio.Low) //nolint:errcheck
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

func (self *LCD) init4() {
	time.Sleep(20 * time.Millisecond)

	// special sequence
	self.Command(0x33)
	self.Command(0x32)

	self.SetFunction(false, true)
	self.SetControl(0) // off
	self.SetControl(ControlOn)
	self.Clear()
	self.SetEntryMode(true, false)
}

func (self *LCD) Command(b Command) {
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
