package telenet

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/helpers/atomic_clock"
	"github.com/temoto/vender/tele"
)

type streamConn struct {
	sync.Mutex
	err  helpers.AtomicError
	last atomic_clock.Clock
	dec  Decoder
	net  net.Conn
	opt  ConnOptions
	stat SessionStat
	w    io.Writer

	vmid   tele.VMID
	authid atomic.Value
}

var _ Conn = &streamConn{}

func NewStreamConn(netConn net.Conn, opt ConnOptions) *streamConn {
	c := &streamConn{
		net: netConn,
		opt: opt,
	}
	if tcp, ok := c.net.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(false)
		_ = tcp.SetLinger(0)
		_ = tcp.SetReadBuffer(16 << 10)
		_ = tcp.SetWriteBuffer(16 << 10)
	}
	const tcpOverhead = 40
	statread := helpers.NewStatReader(c.net, &c.stat.Recv.Total.Size, tcpOverhead)
	c.w = helpers.NewStatWriter(c.net, &c.stat.Send.Total.Size, tcpOverhead)
	c.dec.Attach(bufio.NewReader(statread), opt.ReadLimit)
	c.last.SetNow()
	return c
}

func (c *streamConn) Close() error {
	return c.die(ErrClosing)
}

func (c *streamConn) Closed() bool {
	_, ok := c.err.Load()
	return ok
}

func (c *streamConn) Receive(ctx context.Context) (p *tele.Packet, err error) {
	deadline, _ := ctx.Deadline()
	if err = c.net.SetReadDeadline(deadline); err != nil {
		err = errors.Annotate(err, "SetReadDeadline")
		_ = c.die(err)
		return nil, err
	}
	p, err = c.dec.Read()
	if err != nil {
		err = errors.Annotate(err, "receive")
		_ = c.die(err)
		return nil, err
	}
	c.last.SetNow()
	c.stat.Recv.Register(p)
	return p, nil
}

func (c *streamConn) Send(ctx context.Context, p *tele.Packet) error {
	b, err := FrameMarshal(p)
	if err != nil {
		return errors.Annotate(err, "frame marshal")
	}
	c.opt.Log.Debugf("send p=%s b=(%d)%x", p, len(b), b)
	deadline, _ := ctx.Deadline()
	if err = c.net.SetWriteDeadline(deadline); err != nil {
		err = errors.Annotate(err, "SetWriteDeadline")
		_ = c.die(err)
		return err
	}
	if err = helpers.WriteAll(c.w, b); err != nil {
		err = errors.Annotate(err, "send")
		_ = c.die(err)
		return err
	}
	c.stat.Send.Register(p)
	return nil
}

func (c *streamConn) Options() *ConnOptions        { return &c.opt }
func (c *streamConn) RemoteAddr() net.Addr         { return c.net.RemoteAddr() }
func (c *streamConn) SinceLastRecv() time.Duration { return atomic_clock.Since(&c.last) }
func (c *streamConn) Stat() *SessionStat           { return &c.stat }

func (c *streamConn) ID() (string, tele.VMID) {
	authid, _ := c.authid.Load().(string)
	vmid := tele.VMID(atomic.LoadInt32((*int32)(&c.vmid)))
	return authid, vmid
}

func (c *streamConn) SetID(authid string, vmid tele.VMID) {
	c.authid.Store(authid)
	atomic.StoreInt32((*int32)(&c.vmid), int32(vmid))
}

func (c *streamConn) String() string {
	remote := addrString(c.RemoteAddr())
	authid, vmid := c.ID()
	return fmt.Sprintf("(remote=%s authid=%s vmid=%d)", remote, authid, vmid)
}

func (c *streamConn) die(e error) error {
	if err, found := c.err.StoreOnce(e); found {
		return err
	}
	// c.opt.Log.Debugf("die vmid=%d e=%v", c.VMID(), e)
	// c.alive.Stop()
	_ = c.net.Close()

	// reformat some well known errors for easier log reading
	estr := e.Error()
	if neterr, ok := e.(net.Error); ok && neterr.Timeout() {
		estr = "timeout"
	} else if strings.HasSuffix(estr, "i/o timeout") {
		estr = "timeout"
	} else if strings.HasSuffix(estr, "connection reset by peer") {
		estr = "closed by remote"
	}

	authid, vmid := c.ID()
	c.opt.Log.Debugf("die +close authid=%s vmid=%d local=%s remote=%s e=%s", authid, vmid, addrString(c.net.LocalAddr()), addrString(c.RemoteAddr()), estr)
	return e
}

type Decoder struct {
	buf bytes.Buffer
	r   *bufio.Reader
	max uint32
}

func (d *Decoder) Attach(r *bufio.Reader, max uint32) {
	d.max = max
	d.r = r
}

func (d *Decoder) Read() (*tele.Packet, error) {
	header, err := d.r.Peek(FrameV2HeaderSizeSmall)
	// log.Printf("decoder: Peek header=%x err=%v", header, err)
	switch err {
	case nil:
	case io.EOF:
		if len(header) == 0 {
			return nil, err
		}
		if len(header) <= FrameV2HeaderSizeSmall {
			return nil, errors.Annotate(io.ErrUnexpectedEOF, "header")
		}
	default:
		return nil, errors.Annotate(err, "header")
	}

	magic, frameLen, err := FrameDecode(header, d.max)
	// log.Printf("decoder: FrameDecode magic=%04x frameLen=%d err=%v", magic, frameLen, err)
	if err != nil {
		return nil, errors.Annotate(err, "frame")
	}

	skip := FrameV2HeaderSizeSmall
	switch magic {
	case FrameV2MagicSmall:
	case FrameV2MagicLarge:
		skip = FrameV2HeaderSizeLarge
	default:
		return nil, ErrFrameInvalid
	}
	// log.Printf("decoder: skip=%d", skip)
	if _, err = d.r.Discard(skip); err != nil {
		return nil, errors.Annotate(err, "discard")
	}

	d.buf.Reset()
	d.buf.Grow(int(frameLen))
	buf := d.buf.Bytes()[:frameLen]
	_, err = io.ReadFull(d.r, buf)
	// log.Printf("decoder: readfull buf=%x err=%v", buf, err)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		return nil, errors.Annotate(err, "readfull")
	}
	p := &tele.Packet{}
	if err = proto.Unmarshal(buf, p); err != nil {
		return nil, errors.Annotate(err, "unmarshal")
	}
	return p, nil
}
