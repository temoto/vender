package telenet

import (
	"context"
	"net"
	"net/url"
	"sync/atomic"

	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/helpers/atomic_clock"
	"github.com/temoto/vender/tele"
)

func addrString(a net.Addr) string {
	if a == nil {
		return ""
	}
	return a.String()
}

func parseURI(s string) (scheme, hostport string, err error) {
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return "", "", err
	}
	return u.Scheme, u.Host, nil
}

func nextSeq(addr *uint32) uint32 {
	seq := atomic.AddUint32(addr, 1)
	if atomic.CompareAndSwapUint32(addr, 0, 1) {
		return 1
	}
	return seq
}

type tx struct {
	ctx    context.Context
	f      *helpers.Future
	p      tele.Packet
	stopch <-chan struct{}
	start  int64
}

func newTx(ctx context.Context, p *tele.Packet, stopch <-chan struct{}) *tx {
	tx := &tx{
		ctx:    ctx,
		f:      helpers.NewFuture(),
		p:      *p,
		stopch: stopch,
		start:  atomic_clock.Source(),
	}
	// ensure tx is cancelled with ctx/stopch
	go tx.wait()
	return tx
}

func (tx *tx) wait() interface{} {
	select {
	case <-tx.f.Completed():
	case <-tx.f.Cancelled():
	case <-tx.ctx.Done():
		tx.f.Cancel(context.Canceled)
	case <-tx.stopch:
		tx.f.Cancel(ErrClosing)
	}
	return tx.f.Result()
}
