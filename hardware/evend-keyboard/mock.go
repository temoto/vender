package keyboard

import "time"

type MockKeyboard struct {
	C     chan Key
	Delay time.Duration
}

func NewMockKeyboard(cap int) *MockKeyboard {
	return &MockKeyboard{C: make(chan Key, cap)}
}

func (self *MockKeyboard) Drain() {
	for {
		select {
		case <-self.C:
		default:
			return
		}
	}
}

func (self *MockKeyboard) Wait(timeout time.Duration) (bool, Key) {
	select {
	case key := <-self.C:
		time.Sleep(self.Delay)
		return true, key
	case <-time.After(timeout):
		return false, KeyInvalid
	}
}
