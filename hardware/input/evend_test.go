package input

import (
	"testing"

	"github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/internal/types"
	"github.com/temoto/vender/log2"
)

func TestEvendKeyboard(t *testing.T) {
	t.Parallel()

	mega := &mega.Client{
		TwiChan: make(chan uint16),
	}
	ekb, err := NewEvendKeyboard(mega)
	if err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	d := NewDispatch(log2.NewTest(t, log2.LDebug), stop)
	go d.Run([]Source{ekb})
	inch := d.SubscribeChan("consumer", stop)

	mega.TwiChan <- 0x31
	mega.TwiChan <- 0x31 | 0x80

	e1 := <-inch
	expect1 := types.InputEvent{Source: EvendKeyboardSourceTag, Key: '1', Up: false}
	if e1 != expect1 {
		t.Errorf("input=%#v expect=%#v", e1, expect1)
	}
	select {
	case e2 := <-inch:
		t.Fatalf("unexpected input event=%#v", e2)
	default:
	}
}
