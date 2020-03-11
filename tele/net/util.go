package telenet

import (
	"context"
	"net"
	"net/url"
	"sync/atomic"

	"github.com/temoto/vender/helpers"
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

type delivery struct {
	ctx    context.Context
	p      *tele.Packet
	fu     *helpers.Future
	stopch <-chan struct{}
	acked  uint32
}

func newDelivery(ctx context.Context, p *tele.Packet, stopch <-chan struct{}) *delivery {
	d := &delivery{
		ctx:    ctx,
		fu:     helpers.NewFuture(),
		p:      p,
		stopch: stopch,
	}
	// ensure delivery is cancelled with ctx/stopch
	go d.wait()
	return d
}

func (d *delivery) acked1() bool { return atomic.LoadUint32(&d.acked) >= 1 }
func (d *delivery) acked2() bool { return atomic.LoadUint32(&d.acked) >= 2 }

func (d *delivery) wait() interface{} {
	select {
	case <-d.fu.Completed():
	case <-d.fu.Cancelled():
	case <-d.ctx.Done():
		d.fu.Cancel(context.Canceled)
	case <-d.stopch:
		d.fu.Cancel(ErrClosing)
	}
	return d.fu.Result()
}
