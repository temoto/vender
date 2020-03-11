package telenet

import (
	"context"
	"fmt"
	"net"
	"sync"
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
// - establish connections
// - keepalive pings
// - retry delivery of outgoing packets
type Client struct { //nolint:maligned
	sync.Mutex // protects current and sends
	alive      *alive.Alive
	current    Conn
	opt        *ClientOptions
	stat       SessionStat
	backoff    *helpers.Backoff
}

type ClientOptions struct {
	ConnOptions
	AuthID       string
	VmId         tele.VMID
	BuildVersion string
	Dialer       *net.Dialer
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
		opt: opt,
	}

	go c.pinger()
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

func (c *Client) Send(ctx context.Context, p *tele.Packet) error {
	if !c.alive.Add(1) {
		return ErrClosing
	}
	defer c.alive.Done()

	for {
		conn, err := c.mustConn(ctx)
		if err != nil {
			return err
		}

		if err = conn.Send(ctx, p); err == nil {
			switch err {
			case nil, context.Canceled, ErrClosing:
				return err
			}
			conn.Close()
		}
		// and retry
	}
}

func (c *Client) Stat() *SessionStat { return &c.stat }

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
	hello := NewPacketHello(timestamp, c.opt.AuthID, c.opt.VmId)
	hello.BuildVersion = c.opt.BuildVersion
	if err := conn.Send(ctx, &hello); err != nil {
		return err
	}
	connack, err := conn.Receive(ctx)
	// c.opt.Log.Debugf("client: handshake receive frame=%s err=%v", connack, err)
	if err != nil {
		return err
	}
	if !connack.Hello {
		return conn.die(fmt.Errorf("expected hello response received=%s", connack))
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
	pingpkt := &tele.Packet{Ping: true}
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
			_ = conn.Send(context.Background(), pingpkt)
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
