package lcd

import (
	"fmt"
	"sync"
	"time"
)

func NewMockTextDisplay(width uint16, codepage string, scrollDelay time.Duration) (*TextDisplay, fmt.Stringer) {
	dev := new(MockDevicer)
	display, err := NewTextDisplay(width, codepage, scrollDelay)
	if err != nil {
		// t.Fatal(err)
		panic(err)
	}
	display.dev = dev
	return display, dev
}

type MockDevicer struct {
	mu   sync.Mutex
	l1   []byte
	l2   []byte
	c    Control
	y, x uint8
}

func (self *MockDevicer) Clear() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.l1 = nil
	self.l2 = nil
}

func (self *MockDevicer) Control() Control {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.c
}

func (self *MockDevicer) SetControl(c Control) Control {
	self.mu.Lock()
	defer self.mu.Unlock()
	old := self.c
	self.c = c
	return old
}

func (self *MockDevicer) CursorYX(y, x uint8) bool {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.y, self.x = y, x
	return true
}

func (self *MockDevicer) Write(b []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()
	switch self.y {
	case 1:
		self.l1 = b
	case 2:
		self.l2 = b
	}
	// self.b = append(self.b, b...)
}

func (self *MockDevicer) String() string {
	self.mu.Lock()
	defer self.mu.Unlock()
	return fmt.Sprintf("%s\n%s", string(self.l1), string(self.l2))
}
