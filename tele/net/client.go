package telenet

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/tele"
)

const DefaultRetryDelay = 13 * time.Second

// Vender telemetry client, vending machine side.
// Used in vender and in testing the server.
// Responsible for:
// - keepalive pings
// - redial of stream connections
// - retry delivery of outgoing packets
// Client session actively creates new connectionc.
type Client struct { //nolint:maligned
	lastAck int64 // atomic Packet.Time

	sync.Mutex // protects current and txsend
	alive      *alive.Alive
	current    Conn
	opt        *ClientOptions
	seq        uint32
	stat       SessionStat
	backoff    *helpers.Backoff

	recvs map[uint32]*tx // incoming ack
	sends map[uint32]*tx // outgoing ack
}

type ClientOptions struct {
	ConnOptions
	AuthID       string
	VmId         tele.VMID
	BuildVersion string
	Dialer       *net.Dialer
	Keepalive    time.Duration
	// TODO PacketURL    string
	StreamURL string
}

func NewClient(opt *ClientOptions) (*Client, error) {
	if opt.OnPacket == nil {
		return nil, errors.NotValidf("code error client OnPacket=nil")
	}
	if opt.NetworkTimeout == 0 {
		opt.NetworkTimeout = DefaultNetworkTimeout
	}
	if opt.ReadLimit == 0 {
		opt.ReadLimit = DefaultReadLimit
	}
	if opt.RetryDelay == 0 {
		opt.RetryDelay = DefaultRetryDelay
	}
	opt.Dialer = &net.Dialer{Timeout: opt.NetworkTimeout}
	if opt.Keepalive != 0 {
		opt.Dialer.KeepAlive = -1
	}

	if _, _, err := parseURI(opt.StreamURL); err != nil {
		return nil, errors.Annotatef(err, "config error tele StreamURL=%s", opt.StreamURL)
	}
	c := &Client{
		alive: alive.NewAlive(),
		backoff: &helpers.Backoff{
			Min: opt.RetryDelay,
			Max: 10 * opt.RetryDelay,
			K:   2,
		},
		opt:   opt,
		recvs: make(map[uint32]*tx, 8),
		sends: make(map[uint32]*tx, 8),
	}

	// s.SetID(c.opt.AuthID, c.opt.VmId)

	go c.pinger()
	if !c.alive.Add(1) {
		return nil, ErrClosing
	}
	go c.worker()
	return c, nil
}

func (c *Client) Close() error {
	c.alive.Stop()
	c.Lock()
	conn := c.getConn()
	c.Unlock()
	var err error
	if conn != nil {
		err = conn.Close()
	}
	c.alive.Wait()
	return err
}

func (c *Client) Stat() *SessionStat { return &c.stat }

func (c *Client) Tx(ctx context.Context, p *tele.Packet) error {
	if !c.alive.Add(1) {
		return ErrClosing
	}
	defer c.alive.Done()

	p.Seq = nextSeq(&c.seq)
	tx := newTx(ctx, p, c.alive.StopChan())
	if err := c.txBegin(tx); err != nil {
		return err
	}

	for {
		conn, err := c.mustConn(ctx)
		if err != nil {
			return err
		}

		if err = conn.Send(ctx, p); err == nil {
			err, _ = tx.wait().(error)
			switch err {
			case nil, context.Canceled, ErrClosing:
				return err
			}
			conn.Close()
		}
		// and retry
	}
}

func (c *Client) connect(ctx context.Context) (Conn, error) {
	// TODO try packet connection first

	conn, err := DialContext(ctx, *c.opt.Dialer, c.opt.StreamURL, c.opt.ConnOptions)
	if err != nil {
		return nil, errors.Annotatef(err, "connect stream=%s", c.opt.StreamURL)
	}
	if err = c.handshake(ctx, conn); err != nil {
		c.opt.Log.Errorf("client: handshake err=%v", err)
		return nil, err
	}
	// c.opt.Log.Debugf("client: handshake success")
	return conn, nil
}

// must be called with lock
func (c *Client) getConn() Conn {
	if c.current != nil && c.current.Closed() {
		c.statHook(c.current)
		c.current = nil
	}
	return c.current
}

func (c *Client) handshake(ctx context.Context, conn Conn) error {
	timestamp := time.Now().UnixNano()
	secret := c.opt.GetSecret(c.opt.AuthID)
	hello, err := NewPacketHello(nextSeq(&c.seq), timestamp, c.opt.AuthID, c.opt.VmId, secret)
	if err != nil {
		return err
	}
	hello.BuildVersion = c.opt.BuildVersion
	if err = conn.Send(ctx, &hello); err != nil {
		return err
	}
	connack, err := conn.Receive(ctx)
	// c.opt.Log.Debugf("client: handshake receive p=%s err=%v", connack, err)
	if err != nil {
		return err
	}
	if !connack.Ack || connack.Seq != hello.Seq {
		return conn.die(fmt.Errorf("expected connack received=%s", connack))
	}
	if connack.Error != "" {
		return conn.die(fmt.Errorf("connect denied error=%s", connack.Error))
	}
	// (packet-conn) c.lastRecv.SetNow()
	return nil
}

func (c *Client) mustConn(ctx context.Context) (Conn, error) {
	c.Lock()
	defer c.Unlock()
	if c.current != nil && !c.current.Closed() {
		return c.current, nil
	}

	delay := c.backoff.DelayBefore()
	c.opt.Log.Debugf("reconnect delay=%s", delay)
	if err := c.sleep(ctx, delay); err != nil {
		return nil, err
	}
	if conn, err := c.connect(ctx); err != nil {
		c.backoff.Failure()
		return nil, err
	} else {
		c.backoff.Reset()
		c.statHook(c.current)
		c.current = conn
		return conn, nil
	}
}

func (c *Client) pinger() {
	if c.opt.Keepalive == 0 {
		return
	}
	c.opt.Log.Debugf("pinger keepalive=%s", c.opt.Keepalive)
	for c.alive.IsRunning() {
		c.Lock()
		conn := c.getConn()
		c.Unlock()
		if conn == nil {
			if c.sleep(context.Background(), c.opt.Keepalive/2) != nil {
				return
			}
			continue
		}
		since := conn.SinceLastRecv()
		delay := c.opt.Keepalive - since
		if since > 0 && delay > 0 {
			c.opt.Log.Debugf("pinger since=%s delay=%s -> sleep", since, delay)
			if c.sleep(context.Background(), delay) != nil {
				return
			}
		} else if since > 0 && delay <= 0 {
			c.opt.Log.Debugf("pinger since=%s delay=%s -> send", since, delay)
			// Attempt just single ping delivery over existing connection.
			_ = conn.Send(context.Background(), &tele.Packet{
				Seq:  nextSeq(&c.seq),
				Ping: true,
			})
			if c.sleep(context.Background(), c.opt.NetworkTimeout) != nil {
				return
			}
		} else {
			c.opt.Log.Debugf("pinger since=%s delay=%s -> again", since, delay)
		}
	}
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil

	case <-ctx.Done():
		return context.Canceled

	case <-c.alive.StopChan():
		return ErrClosing
	}
}

func (c *Client) statHook(conn Conn) {
	if conn != nil {
		c.stat.AddMoveFrom(conn.Stat())
	}
}

func (c *Client) worker() {
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

func (c *Client) workerStep(ctx context.Context, timeout time.Duration) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, _ := c.mustConn(ctx)
	if conn == nil {
		return
	}

	p, err := conn.Receive(ctx)
	c.statHook(conn)
	if err == nil {
		// (packet-conn) c.lastRecv.SetNow()
		go c.processPacket(ctx, conn, p)
		return
	}
}

func (c *Client) locked_cleanAck(timestamp int64) {
	if timestamp == 0 {
		return
	}
	for {
		lastAck := atomic.LoadInt64(&c.lastAck)
		if timestamp < lastAck {
			return
		}
		if atomic.CompareAndSwapInt64(&c.lastAck, lastAck, timestamp) {
			break
		}
	}
	cutoff := timestamp - (int64(c.opt.NetworkTimeout) * 3)
	for seq, tx := range c.sends {
		if tx.f.Result() != nil && tx.start <= cutoff {
			delete(c.sends, seq)
		}
	}
}

func (c *Client) processAck(p *tele.Packet) {
	c.Lock()
	defer c.Unlock()

	tx := c.sends[p.Seq]
	if tx == nil {
		c.opt.Log.Errorf("unexpected ack seq=%d", p.Seq)
		return
	}

	tx.f.Complete(p.Error)
	c.locked_cleanAck(tx.p.Time)
	if p.Error != "" {
		c.opt.Log.Errorf("processAck seq=%d p.error=%s", p.Seq, p.Error)
	}
}

func (c *Client) processPacket(ctx context.Context, pconn Conn, p *tele.Packet) {
	if p.Ack {
		c.processAck(p)
		return
	}

	authid, _ := pconn.ID()
	err := c.opt.OnPacket(authid, p)
	if err != nil {
		c.opt.Log.Error(errors.Annotate(err, "OnPacket"))
		return
	}
	pack := NewPacketAck(p)
	pack.Time = time.Now().UnixNano()
	conn, err := c.mustConn(ctx)
	if err == nil {
		err = conn.Send(ctx, &pack)
	}
	if err != nil {
		c.opt.Log.Error(errors.Annotate(err, "ack"))
	}
}

func (c *Client) txBegin(tx *tx) error {
	c.Lock()
	defer c.Unlock()
	if !c.alive.IsRunning() {
		return ErrClosing
	}
	if _, ok := c.sends[tx.p.Seq]; ok {
		err := fmt.Errorf("dup seq=%d", tx.p.Seq)
		tx.f.Cancel(err)
		return err
	}
	c.sends[tx.p.Seq] = tx
	return nil
}
