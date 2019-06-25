package input

import (
	"testing"

	"github.com/temoto/vender/log2"
)

func TestDispatchDoubleSubscribe(t *testing.T) {
	log := log2.NewTest(t, log2.LDebug)
	dstop := make(chan struct{})
	d := NewDispatch(log, dstop)

	go func() {
		sub1stop := make(chan struct{})
		d.SubscribeChan("name", sub1stop)
		close(sub1stop)
		sub2stop := make(chan struct{})
		d.SubscribeChan("name", sub2stop)
		close(dstop)
	}()

	d.Run(nil)
}
