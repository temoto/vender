package inputevent

import (
	"fmt"
	"io"
	"unsafe"
)

func parse(b [EventSizeof]byte) (InputEvent, error) {
	// Sorry for not using proper serialization.
	return *(*InputEvent)(unsafe.Pointer(&b[0])), nil
}

func ReadOne(r io.Reader) (InputEvent, error) {
	var bufa [EventSizeof]byte

	n, err := r.Read(bufa[:])
	if err != nil {
		return InputEvent{}, err
	}
	if n < EventSizeof {
		return InputEvent{}, fmt.Errorf("read n=%d expected=%d", n, EventSizeof)
	}

	return parse(bufa)
}

func ReadChan(r io.Reader, ch chan<- InputEvent) error {
	for {
		var event InputEvent
		event, err := ReadOne(r)
		if err != nil {
			return err
		}
		ch <- event
	}
}
