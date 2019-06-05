package keyboard

import (
	"time"

	"github.com/temoto/vender/hardware/mega-client"
)

const KeyMaskUp = 0x80

type Key uint16

const (
	KeyInvalid   Key = 0
	KeyAccept    Key = 13
	KeyReject    Key = 27
	KeyCreamLess Key = 'A'
	KeyCreamMore Key = 'B'
	KeySugarLess Key = 'C'
	KeySugarMore Key = 'D'
	KeyDot       Key = 'E'
)

type Inputer interface {
	Drain()
	Wait(timeout time.Duration) (bool, Key)
}

type Keyboard struct {
	// mu     sync.Mutex
	c      *mega.Client
	source <-chan uint16
	// pressed [128]bool
}

func NewKeyboard(client *mega.Client) (*Keyboard, error) {
	self := &Keyboard{c: client}
	self.c.IncRef("ui-keyboard")
	self.source = self.c.TwiChan

	// drain buffered key events
	self.Drain()
	return self, nil
}
func (self *Keyboard) Close() error {
	self.source = nil
	return self.c.DecRef("ui-keyboard")
}

func (self *Keyboard) Drain() {
	for {
		select {
		case <-self.source:
		default:
			return
		}
	}
}

func (self *Keyboard) Wait(timeout time.Duration) (bool, Key) {
	v, ok := self.wait(timeout)
	if !ok {
		return false, KeyInvalid
	}
	return self.parse(v)
}

func WaitUp(kb Inputer, timeout time.Duration) Key {
	for {
		up, key := kb.Wait(timeout)
		// log.Printf("keyboard.WaitUp=%t,%x", up, key)
		if key == KeyInvalid {
			return KeyInvalid
		}
		if up {
			return key
		}
	}
}

func (self *Keyboard) parse(v16 uint16) (bool, Key) {
	value := Key(v16)
	up := value&KeyMaskUp != 0
	key := value &^ KeyMaskUp
	// self.mu.Lock()
	// self.pressed[key] = up
	// self.mu.Unlock()
	return up, key
}

func (self *Keyboard) wait(timeout time.Duration) (uint16, bool) {
	select {
	case v := <-self.source:
		return v, true
	case <-time.After(timeout):
		return 0, false
	}
}
