package lcd

import (
	"github.com/nathan-osman/go-rpigpio"
	"github.com/paulrosania/go-charset/charset"
	_ "github.com/paulrosania/go-charset/data"

	"time"
)

type LCD struct {
	pin_u0  *rpi.Pin
	pin_a0  *rpi.Pin
	pin_e   *rpi.Pin
	pin_db4 *rpi.Pin
	pin_db5 *rpi.Pin
	pin_db6 *rpi.Pin
	pin_db7 *rpi.Pin
}

func (lcd *LCD) Init() (err error) {
	// violet
	lcd.pin_u0, err = rpi.OpenPin(18, rpi.OUT)
	if err != nil {
		return
	}
	// blue
	lcd.pin_a0, err = rpi.OpenPin(23, rpi.OUT)
	if err != nil {
		return
	}
	// green
	lcd.pin_e, err = rpi.OpenPin(24, rpi.OUT)
	if err != nil {
		return
	}
	// yellow
	lcd.pin_db4, err = rpi.OpenPin(7, rpi.OUT)
	if err != nil {
		return
	}
	// orange
	lcd.pin_db5, err = rpi.OpenPin(8, rpi.OUT)
	if err != nil {
		return
	}
	// red
	lcd.pin_db6, err = rpi.OpenPin(9, rpi.OUT)
	if err != nil {
		return
	}
	// brown
	lcd.pin_db7, err = rpi.OpenPin(10, rpi.OUT)
	if err != nil {
		return
	}
	lcd.setAllPins(false)

	return nil
}

func setPinBool(pin *rpi.Pin, value bool) error {
	var v rpi.Value = rpi.LOW
	if value {
		v = rpi.HIGH
	}
	return pin.Write(v)
}

func (lcd *LCD) setAllPins(b bool) {
	setPinBool(lcd.pin_u0, b)
	setPinBool(lcd.pin_a0, b)
	setPinBool(lcd.pin_e, b)
	setPinBool(lcd.pin_db4, b)
	setPinBool(lcd.pin_db5, b)
	setPinBool(lcd.pin_db6, b)
	setPinBool(lcd.pin_db7, b)
}

func (lcd *LCD) blinkE() {
	setPinBool(lcd.pin_e, true)
	time.Sleep(1 * time.Microsecond)
	setPinBool(lcd.pin_e, false)
	time.Sleep(1 * time.Microsecond)
}

func (lcd *LCD) send4(a0, db4, db5, db6, db7 bool) {
	// fmt.Printf("sn4 %v %v %v %v %v\n", a0, db7, db6, db5, db4)
	setPinBool(lcd.pin_a0, a0)
	setPinBool(lcd.pin_db4, db4)
	setPinBool(lcd.pin_db5, db5)
	setPinBool(lcd.pin_db6, db6)
	setPinBool(lcd.pin_db7, db7)
	lcd.blinkE()
}

func (lcd *LCD) Init4() {
	time.Sleep(20 * time.Millisecond)

	// special sequence
	lcd.Command(0x33)
	lcd.Command(0x32)

	lcd.CommandFunction(false, true)
	lcd.CommandOff()
	lcd.CommandOn(true, false, false)
	lcd.CommandClear()
	lcd.CommandEntryMode(true, false)
}

func (lcd *LCD) Command(b byte) {
	// fmt.Printf("cmd %0x\n", b)
	lcd.send4(false, b&(1<<4) != 0, b&(1<<5) != 0, b&(1<<6) != 0, b&(1<<7) != 0)
	lcd.send4(false, b&(1<<0) != 0, b&(1<<1) != 0, b&(1<<2) != 0, b&(1<<3) != 0)
	time.Sleep(40 * time.Microsecond)
	lcd.setAllPins(false)
}

func (lcd *LCD) Data(b byte) {
	// fmt.Printf("dat %0x\n", b)
	lcd.send4(true, b&(1<<4) != 0, b&(1<<5) != 0, b&(1<<6) != 0, b&(1<<7) != 0)
	lcd.send4(true, b&(1<<0) != 0, b&(1<<1) != 0, b&(1<<2) != 0, b&(1<<3) != 0)
	time.Sleep(40 * time.Microsecond)
	lcd.setAllPins(false)
}

func (lcd *LCD) WriteString(s string, sleepChar, sleepWord int) {
	tr, _ := charset.TranslatorTo("windows-1251")
	_, s1251, _ := tr.Translate([]byte(s), true)

	for _, r := range s1251 {
		if r != '\n' {
			lcd.Data(byte(r))
		} else {
			lcd.CommandAddress(0x40)
		}
		if r == ' ' && sleepWord > 0 {
			time.Sleep(time.Duration(sleepWord) * time.Millisecond)
		} else if sleepChar > 0 {
			time.Sleep(time.Duration(sleepChar) * time.Millisecond)
		}
	}
}

func (lcd *LCD) CommandClear() {
	lcd.Command(0x01)
	time.Sleep(2 * time.Millisecond)
}

func (lcd *LCD) CommandReturn() {
	lcd.Command(0x02)
}

func (lcd *LCD) CommandEntryMode(right, shift bool) {
	var cmd byte = 0x04
	if right {
		cmd |= 0x02
	}
	if shift {
		cmd |= 0x01
	}
	lcd.Command(cmd)
}

func (lcd *LCD) CommandOn(on, underscore, blink bool) {
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
	lcd.Command(cmd)
}

func (lcd *LCD) CommandOff() {
	lcd.Command(0x08)
}

func (lcd *LCD) CommandFunction(bits8, page1 bool) {
	var cmd byte = 0x28
	if bits8 {
		cmd |= 0x10
	}
	if page1 {
		cmd |= 0x02
	}
	lcd.Command(cmd)
}

func (lcd *LCD) CommandAddress(a byte) {
	lcd.Command(0x80 | a)
}
