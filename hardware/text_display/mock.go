package text_display

func NewMockTextDisplay(opt *TextDisplayConfig) *TextDisplay {
	dev := new(MockDevicer)
	display, err := NewTextDisplay(opt)
	if err != nil {
		// t.Fatal(err)
		panic(err)
	}
	display.dev = dev
	return display
}

type MockDevicer struct {
	c uint32
}

func (self *MockDevicer) Clear() {}

// func (self *MockDevicer) Control() Control {
// 	return Control(atomic.LoadUint32(&self.c))
// }

func (self *MockDevicer) CursorYX(y, x uint8) bool { return true }

// func (self *MockDevicer) SetControl(c Control) Control {
// 	return Control(atomic.SwapUint32(&self.c, uint32(c)))
// }

func (self *MockDevicer) Write(b []byte) {}
