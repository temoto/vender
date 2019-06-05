package state

import (
	"os"
	"time"

	"github.com/temoto/inputevent-go"
	keyboard "github.com/temoto/vender/hardware/evend-keyboard"
)

type InputKind byte

const (
	InputNothing InputKind = iota
	InputNormal
	InputAccept
	InputReject
	InputOther
)

type InputEvent struct {
	Kind InputKind
	Key  keyboard.Key // FIXME make keyboard use input.Key type, not reverse
}

func InputDrain(ch <-chan InputEvent) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func newInputEvents(g *Global, stopch <-chan struct{}) <-chan InputEvent {
	ch := make(chan InputEvent)

	// support more input sources here
	go feedKeyboardEvend(g, ch)

	return ch
}

func feedKeyboardEvend(g *Global, ch chan<- InputEvent) {
	const timeout = time.Second
	kb := g.Hardware.Keyboard.Device
	kb.Drain()
	stopch := g.Alive.StopChan()

	for g.Alive.IsRunning() {
		key := keyboard.WaitUp(kb, timeout)
		// g.Log.Debugf("key %02[1]x=%[1]d=%[1]q", key)
		event := parseKey(key)
		if event.Kind != InputNothing {
			select {
			case <-stopch:
				return
			case ch <- event:
			}
		}
	}
}

func feedKeyboardLinux(g *Global, ch chan<- InputEvent) {
	// FIXME config
	f, err := os.Open("/dev/input/event0")
	if err != nil {
		g.Log.Errorf("linux-input err=%v", err)
		return
	}
	defer f.Close()

	go func() {
		g.Alive.Wait()
		f.Close() // to unblock `ReadOne(f)`
	}()
	for g.Alive.IsRunning() {
		var event inputevent.InputEvent
		event, err := inputevent.ReadOne(f)
		if err != nil {
			g.Log.Errorf("linux-input err=%v", err)
			return
		}
		if (event.Type == inputevent.EV_KEY) && (event.Value == int32(inputevent.KeyStateUp)) {
			g.Log.Debugf("linux-input key=%v", event.Code)
			ch <- InputEvent{
				Kind: InputOther,
				Key:  keyboard.Key(event.Code),
			}
		}
	}
}

func parseKey(key keyboard.Key) InputEvent {
	switch key {
	case keyboard.KeyInvalid:
		return InputEvent{Kind: InputNothing}
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return InputEvent{Kind: InputNormal, Key: key}
	case keyboard.KeyAccept:
		return InputEvent{Kind: InputAccept, Key: key}
	case keyboard.KeyReject:
		return InputEvent{Kind: InputReject, Key: key}
	default:
		return InputEvent{Kind: InputOther, Key: key}
	}
}
