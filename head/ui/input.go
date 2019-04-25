package ui

import (
	"context"
	"time"

	keyboard "github.com/temoto/vender/hardware/evend-keyboard"
	"github.com/temoto/vender/state"
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
	Key  keyboard.Key
}

func InputEvents(ctx context.Context, stopch <-chan struct{}) <-chan InputEvent {
	config := state.GetConfig(ctx)

	// support more input sources here
	kb := config.Global().Hardware.Keyboard.Device
	kb.Drain()

	ch := make(chan InputEvent)
	go func() {
		const timeout = time.Second

		for {
			key := kb.WaitUp(timeout)
			// log.Printf("key %02[1]x=%[1]d=%[1]q", key)
			event := parseKey(key)
			if event.Kind == InputNothing {
				select {
				case <-stopch:
					close(ch)
					return
				default:
				}
			} else {
				select {
				case <-stopch:
					close(ch)
					return
				case ch <- event:
				}
			}
		}
	}()
	return ch
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
