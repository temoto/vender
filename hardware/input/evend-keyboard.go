package input

import (
	"io"

	"github.com/temoto/vender/hardware/mega-client"
)

const EvendKeyMaskUp = 0x80
const EvendKeyboardSourceTag = "evend-keyboard"

const (
	EvendKeyAccept    Key = 13
	EvendKeyReject    Key = 27
	EvendKeyCreamLess Key = 'A'
	EvendKeyCreamMore Key = 'B'
	EvendKeySugarLess Key = 'C'
	EvendKeySugarMore Key = 'D'
	evendKeyDotInput  Key = 'E' // evend keyboard sends '.' as 'E'
	EvendKeyDot       Key = '.'
)

type EvendKeyboard struct{ c *mega.Client }

// compile-time interface compliance test
var _ Source = new(EvendKeyboard)

func NewEvendKeyboard(client *mega.Client) (*EvendKeyboard, error) {
	self := &EvendKeyboard{c: client}
	self.c.IncRef(EvendKeyboardSourceTag)

drain:
	for {
		select {
		case <-self.c.TwiChan:
		default:
			break drain
		}
	}
	return self, nil
}
func (self *EvendKeyboard) Close() error {
	return self.c.DecRef(EvendKeyboardSourceTag)
}

func (self *EvendKeyboard) String() string { return EvendKeyboardSourceTag }

func (self *EvendKeyboard) Read() (Event, error) {
	for {
		v16, ok := <-self.c.TwiChan
		if !ok {
			return Event{}, io.EOF
		}
		key, up := Key(v16&^EvendKeyMaskUp), v16&EvendKeyMaskUp != 0
		// key replace table
		switch key {
		case evendKeyDotInput:
			key = EvendKeyDot
		}
		if !up {
			e := Event{
				Source: EvendKeyboardSourceTag,
				Key:    key,
				Up:     up,
			}
			return e, nil
		}
	}
}

func IsAccept(e *Event) bool { return e.Source == EvendKeyboardSourceTag && e.Key == EvendKeyAccept }
func IsReject(e *Event) bool { return e.Source == EvendKeyboardSourceTag && e.Key == EvendKeyReject }
