package telenet

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/helpers/atomic_clock"
	"github.com/temoto/vender/tele"
)

type streamConn struct {
	sync.Mutex
	alive  *alive.Alive
	stat   SessionStat
	acks   ackmap
	err    helpers.AtomicError
	last   atomic_clock.Clock
	dec    Decoder
	net    net.Conn
	opt    ConnOptions
	w      io.Writer
	authid atomic.Value
	vmid   tele.VMID
	seq    uint32
}

var _ Conn = &streamConn{}

func NewStreamConn(netConn net.Conn, opt ConnOptions) *streamConn {
	c := &streamConn{
		alive: alive.NewAlive(), // TODO link to parent
		net:   netConn,
		opt:   opt,
	}
	c.acks.m = make(map[uint16]chan struct{})

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

	if !c.alive.Add(1) {
		return nil
	}
	go c.worker()
	return c
}

func (c *streamConn) Close() error {
	return c.die(ErrClosing)
}

func (c *streamConn) Closed() bool {
	_, ok := c.err.Load()
	return ok
}

func (c *streamConn) Send(ctx context.Context, p *tele.Packet) error {
	seq := seqNext(&c.seq)
	f := NewFrame(seq, p)
	authid, _ := c.ID()
	secret, err := c.opt.GetSecret(authid, f)
	if err != nil {
		return errors.Annotate(err, "getsecret")
	}
	b, err := FrameMarshal(f, secret)
	if err != nil {
		return errors.Annotate(err, "frame marshal")
	}
	c.opt.Log.Debugf("send f=%s b=(%d)%x", f, len(b), b)
	deadline, _ := ctx.Deadline()
	if err = c.net.SetWriteDeadline(deadline); err != nil {
		err = errors.Annotate(err, "SetWriteDeadline")
		_ = c.die(err)
		return err
	}
	ackch := c.acks.Register(seq)
	if err = helpers.WriteAll(c.w, b); err != nil {
		err = errors.Annotate(err, "send")
		_ = c.die(err)
		return err
	}
	c.stat.Send.Register(f.Packet)

	select {
	case <-ackch:
		return nil

	case <-ctx.Done():
		c.acks.Cancel(seq)
		return context.Canceled

	case <-c.alive.StopChan():
		c.acks.Cancel(seq)
		return ErrClosing
	}
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

func (c *streamConn) receive(ctx context.Context) (*tele.Frame, error) {
	deadline, _ := ctx.Deadline()
	if err := c.net.SetReadDeadline(deadline); err != nil {
		err = errors.Annotate(err, "SetReadDeadline")
		_ = c.die(err)
		return nil, err
	}
	f, err := c.dec.Read()
	if err != nil {
		err = errors.Annotate(err, "receive")
		_ = c.die(err)
		return nil, err
	}
	c.stat.Recv.Register(f.Packet)
	return f, nil
}

func (c *streamConn) worker() {
	defer c.alive.Done()
	stopch := c.alive.StopChan()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-stopch
		cancel()
	}()
	timeout := c.opt.NetworkTimeout + c.opt.Keepalive

	for c.alive.IsRunning() {
		c.workerStep(ctx, timeout)
	}
}

func (c *streamConn) workerStep(ctx context.Context, timeout time.Duration) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	f, err := c.receive(ctx)
	if err == nil {
		// (packet-conn) c.lastRecv.SetNow()
		go c.onRecv(nil, f)
	}
}

func (c *streamConn) onRecv(conn Conn, f *tele.Frame) {
	if f.Packet != nil {
		if err := c.opt.OnPacket(c, f.Packet); err != nil {
			_ = c.die(err)
		}
	}
	c.last.SetNow()
	c.acks.Receive(uint16(f.Sack), f.Acks)
}

type Decoder struct {
	buf bytes.Buffer
	r   *bufio.Reader
	max uint32

	GetSecret func(*tele.Frame) ([]byte, error)
}

func (d *Decoder) Attach(r *bufio.Reader, max uint32) {
	d.max = max
	d.r = r
}

func (d *Decoder) Read() (*tele.Frame, error) {
	header, err := d.r.Peek(FrameV2HeaderSize)
	// log.Printf("decoder: Peek header=%x err=%v", header, err)
	switch err {
	case nil:
	case io.EOF:
		if len(header) == 0 {
			return nil, err
		}
		if len(header) < FrameV2HeaderSize {
			return nil, errors.Annotate(io.ErrUnexpectedEOF, "header")
		}
	default:
		return nil, errors.Annotate(err, "header")
	}

	if _, err = d.r.Discard(FrameV2HeaderSize); err != nil {
		return nil, errors.Annotate(err, "header discard")
	}

	if magic := binary.BigEndian.Uint16(header[0:]); magic != FrameV2Magic {
		return nil, errors.Annotate(ErrFrameInvalid, "wrong magic")
	}
	frameLen := binary.BigEndian.Uint16(header[2:])
	if frameLen < FrameV2HeaderSize {
		return nil, errors.Annotatef(ErrFrameInvalid, "frameLen=%d too small", frameLen)
	}
	if uint32(frameLen) > d.max {
		return nil, errors.Errorf("frameLen=%d exceeds max=%d", frameLen, d.max)
	}
	d.buf.Reset()
	d.buf.Grow(int(frameLen))
	buf := d.buf.Bytes()[:frameLen]
	copy(buf, header)
	_, err = io.ReadFull(d.r, buf[FrameV2HeaderSize:])
	// log.Printf("decoder: readfull buf=(%d)%x err=%v", len(buf), buf, err)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		return nil, errors.Annotate(err, "frame body")
	}

	frame := &tele.Frame{}
	if err = FrameUnmarshal(buf, frame, d.GetSecret); err != nil {
		return nil, err
	}
	return frame, nil
}
