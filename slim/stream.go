package slim

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

	"github.com/juju/errors"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/helpers/atomic_clock"
	"github.com/temoto/vender/tele"
)

type streamConn struct {
	sync.Mutex
	alive *alive.Alive
	stat  SessionStat
	acks  ackmap
	err   helpers.AtomicError
	last  atomic_clock.Clock
	dec   Decoder
	net   net.Conn
	opt   ConnOptions
	w     io.Writer
	id    atomic.Value
	vmid  tele.VMID
	seq   uint32
}

var _ Conn = &streamConn{}

func NewStreamConn(netConn net.Conn, opt ConnOptions) (*streamConn, error) {
	if opt.OnPayload == nil {
		return nil, errors.NotValidf("code error NewStreamConn opt.OnPayload=nil")
	}
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
		return nil, ErrClosing
	}
	go c.worker()
	return c, nil
}

func (c *streamConn) Close() error {
	return c.die(ErrClosing)
}

func (c *streamConn) Closed() bool {
	_, ok := c.err.Load()
	return ok
}

func (c *streamConn) Send(ctx context.Context, payload []byte) error {
	seq := seqNext(&c.seq)
	f := &Frame{
		Seq:     seq,
		Payload: payload,
	}
	b, err := f.Marshal()
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
	c.stat.Send.Register(f)

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

func (c *streamConn) Done() <-chan struct{}        { return c.alive.WaitChan() }
func (c *streamConn) ID() ID                       { return c.id.Load() }
func (c *streamConn) Options() *ConnOptions        { return &c.opt }
func (c *streamConn) RemoteAddr() net.Addr         { return c.net.RemoteAddr() }
func (c *streamConn) SinceLastRecv() time.Duration { return atomic_clock.Since(&c.last) }
func (c *streamConn) Stat() *SessionStat           { return &c.stat }
func (c *streamConn) SetID(id ID)                  { c.id.Store(id) }

func (c *streamConn) String() string {
	remote := addrString(c.RemoteAddr())
	id := c.ID()
	return fmt.Sprintf("(remote=%s id=%v)", remote, id)
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

	c.opt.Log.Debugf("die +close id=%v local=%s remote=%s e=%s", c.ID(), addrString(c.net.LocalAddr()), addrString(c.RemoteAddr()), estr)
	return e
}

func (c *streamConn) receive(ctx context.Context) (*Frame, error) {
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
	c.stat.Recv.Register(f)
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

func (c *streamConn) onRecv(conn Conn, f *Frame) {
	if f.Payload != nil {
		if err := c.opt.OnPayload(c, f.Payload); err != nil {
			_ = c.die(err)
		}
	}
	c.last.SetNow()
	c.acks.Receive(f.AckSeq, f.Acks)
}

type Decoder struct {
	buf bytes.Buffer
	r   *bufio.Reader
	max uint16

	GetSecret GetSecretFunc
	OpaqueID  ID
}

func (d *Decoder) Attach(r *bufio.Reader, max uint16) {
	d.max = max
	d.r = r
}

func (d *Decoder) Read() (*Frame, error) {
	header, err := d.r.Peek(V2HeaderFixed)
	// log.Printf("decoder: Peek header=%x err=%v", header, err)
	switch err {
	case nil:
	case io.EOF:
		if len(header) == 0 {
			return nil, err
		}
		if len(header) < V2HeaderFixed {
			return nil, errors.Annotate(io.ErrUnexpectedEOF, "header")
		}
	default:
		return nil, errors.Annotate(err, "header")
	}

	if _, err = d.r.Discard(V2HeaderFixed); err != nil {
		return nil, errors.Annotate(err, "header discard")
	}

	frame := &Frame{
		GetSecret: d.GetSecret,
		OpaqueID:  d.OpaqueID,
	}
	if err = frame.DecodeFixedHeader(header); err != nil {
		return nil, err
	}
	if frame.length > d.max {
		return nil, errors.Errorf("frame length=%d exceeds max=%d", frame.length, d.max)
	}
	d.buf.Reset()
	d.buf.Grow(int(frame.length))
	buf := d.buf.Bytes()[:frame.length]
	copy(buf, header)
	_, err = io.ReadFull(d.r, buf[V2HeaderFixed:])
	// log.Printf("decoder: readfull buf=(%d)%x err=%v", len(buf), buf, err)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		return nil, errors.Annotate(err, "frame body")
	}

	if err = frame.Unmarshal(buf); err != nil {
		return nil, err
	}
	return frame, nil
}
