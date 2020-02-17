package mqtt

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/256dpi/gomqtt/client"
	"github.com/256dpi/gomqtt/client/future"
	"github.com/256dpi/gomqtt/packet"
	"github.com/256dpi/gomqtt/transport"
	"github.com/juju/errors"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/helpers/atomic_clock"
	"github.com/temoto/vender/log2"
)

const DefaultNetworkTimeout = 30 * time.Second
const DefaultReconnectDelay = 3 * time.Second

var ErrClientClosing = fmt.Errorf("MQTT client is closing")

type ClientOptions struct {
	BrokerURL      string
	TLS            *tls.Config
	ReconnectDelay time.Duration
	NetworkTimeout time.Duration
	KeepaliveSec   uint16
	ClientID       string
	Username       string
	Password       string
	Subscriptions  []packet.Subscription
	OnMessage      func(*packet.Message) error
	Will           *packet.Message
	Log            *log2.Log

	conpkt   *packet.Connect
	dialer   *transport.Dialer
	onpacket func(*clientConn, packet.Generic)
}

// Vender telemetry specific MQTT client.
// - NewClient() returns only configuration errors, network IO is done in background
// - Connect with clean session only
// - Subscribe for configured list, no unsubscribe
// - Unlimited reconnect attempts until Close()
// - QOS 0,1
// - No in-flight storage (except Publish call stack)
// - Serialized Publish
// - Publish while offline returns ErrClientNotConnected
type Client struct { //nolint:maligned
	sync.Mutex

	alive   *alive.Alive
	current *clientConn
	lastID  uint32
	opt     ClientOptions

	flowPublish struct {
		sync.Mutex
		fu *future.Future
		id packet.ID
	}
}

func NewClient(opt ClientOptions) (*Client, error) {
	if opt.OnMessage == nil {
		return nil, errors.NotValidf("code error mqtt.ClientOptions.OnMessage=nil")
	}
	if opt.NetworkTimeout == 0 {
		opt.NetworkTimeout = DefaultNetworkTimeout
	}
	if opt.ReconnectDelay == 0 {
		opt.ReconnectDelay = DefaultReconnectDelay
	}
	if u, err := url.ParseRequestURI(opt.BrokerURL); err != nil {
		return nil, errors.Annotatef(err, "config error mqtt BrokerURL=%s", opt.BrokerURL)
	} else if u.User != nil && opt.Username == "" && opt.Password == "" {
		opt.Username = u.User.Username()
		opt.Password, _ = u.User.Password()
	}
	opt.conpkt = packet.NewConnect()
	opt.conpkt.ClientID = defaultString(opt.ClientID, opt.Username)
	opt.conpkt.KeepAlive = uint16(opt.KeepaliveSec)
	opt.conpkt.CleanSession = true
	opt.conpkt.Username = opt.Username
	opt.conpkt.Password = opt.Password
	opt.conpkt.Will = opt.Will
	opt.dialer = transport.NewDialer(transport.DialConfig{
		TLSConfig: opt.TLS,
		Timeout:   opt.NetworkTimeout,
	})

	// opt.Log.Debugf("effective options=%#v", &opt)
	c := &Client{
		alive:  alive.NewAlive(),
		lastID: uint32(time.Now().UnixNano()),
		opt:    opt,
	}
	c.opt.onpacket = c.onPacket
	_ = c.clientConn(true)

	go c.worker()
	return c, nil
}

func (c *Client) Close() error {
	err := c.Disconnect()
	c.alive.Stop()
	c.alive.Wait()
	return err
}

func (c *Client) Disconnect() error {
	err := client.ErrClientNotConnected
	if cc := c.clientConn(false); cc != nil {
		err = cc.send(packet.NewDisconnect())
		err = cc.die(err)
	}
	return err
}

func (c *Client) Publish(ctx context.Context, msg *packet.Message) error {
	if msg.QOS >= packet.QOSExactlyOnce {
		panic("code error QOS ExactlyOnce not implemented")
	}

	// TODO loop try lock flowPublish with IsReady() && !ctx.Done && pubfu=nil
	f, err := c.publishBegin(ctx, msg)
	if err != nil {
		return err
	}

	switch err = f.Wait(c.opt.NetworkTimeout); err {
	case nil:
		return nil

	case future.ErrCanceled:
		return c.flowPublish.fu.Result().(error)

	case future.ErrTimeout:
		// TODO resend with DUP
		err = errors.Timeoutf("Publish ack")
		c.flowPublish.fu.Cancel(err)
		return c.disconnect(err)

	default:
		return fmt.Errorf("code error future.Wait()=%v", err)
	}
}

// Returns, in this order:
// - ErrClosing if client stopped with Close()
// - nil if connected and subscribed within context limit
// - context.Canceled if context canceled/expired before successful connection
func (c *Client) WaitReady(ctx context.Context) error {
	donech := ctx.Done()
	stopch := c.alive.StopChan()
	for {
		cc := c.clientConn(false)
		if cc == nil {
			select {
			case <-time.After(100 * time.Millisecond):
				continue

			case <-donech:
				return context.Canceled

			case <-stopch:
				return ErrClientClosing
			}
		}

		switch cc.waitReady(ctx) {
		case nil: // success path
			return nil

		case context.Canceled:
			return context.Canceled

		case ErrClientClosing: // current connection is lost, just try again
		}
	}
}

func (c *Client) clientConn(create bool) *clientConn {
	c.Lock()
	defer c.Unlock()
	if !c.alive.IsRunning() {
		return nil
	}
	if c.current != nil && !c.current.alive.IsRunning() {
		c.current = nil
	}
	if c.current == nil && create {
		var subpkt *packet.Subscribe
		if len(c.opt.Subscriptions) != 0 {
			subpkt = &packet.Subscribe{
				ID:            c.nextID(),
				Subscriptions: c.opt.Subscriptions,
			}
		}
		c.current = newClientConn(c.opt, subpkt)
	}
	return c.current
}

func (c *Client) disconnect(err error) error {
	if cc := c.clientConn(false); cc != nil {
		_ = cc.die(err)
		// if connErr != nil && !isClosedConn(connErr) {
		// 	c.opt.Log.Errorf("conn.Close err=%v", connErr)
		// 	if err == nil {
		// 		err = connErr
		// 	}
		// }
		cc.alive.Wait()
	}
	return err
}

// Retry on err client.ErrClientNotConnected or future.ErrTimeout
func (c *Client) publishBegin(ctx context.Context, msg *packet.Message) (*future.Future, error) {
	if err := c.WaitReady(ctx); err != nil {
		return nil, err
	}
	c.flowPublish.Lock()
	defer c.flowPublish.Unlock()
	if fprev := c.flowPublish.fu; fprev != nil {
		if err := fprev.Wait(1); err == future.ErrTimeout {
			return nil, err
		}
	}

	publish := packet.NewPublish()
	publish.Message = *msg
	if msg.QOS >= packet.QOSAtLeastOnce {
		publish.ID = c.nextID()
	}

	err := c.send(publish)
	if err != nil {
		return nil, errors.Annotate(err, "send PUBLISH")
	}

	c.flowPublish.fu = future.New()
	c.flowPublish.id = publish.ID
	if msg.QOS == packet.QOSAtMostOnce {
		c.flowPublish.fu.Complete(nil)
	}
	return c.flowPublish.fu, nil
}

func (c *Client) nextID() packet.ID {
	u32 := atomic.AddUint32(&c.lastID, 1)
	return packet.ID(u32 % (1 << 16))
}

func (c *Client) onPacket(conn *clientConn, p packet.Generic) {
	switch pt := p.(type) {
	case *packet.Publish:
		c.onPublish(pt)
	case *packet.Puback:
		c.onPuback(pt.ID)
	default:
		c.opt.Log.Debugf("unknown packet %s", PacketString(p))
	}
}

func (c *Client) onPublish(publish *packet.Publish) {
	// call callback for unacknowledged and directly acknowledged messages
	if publish.Message.QOS <= packet.QOSAtLeastOnce {
		err := c.opt.OnMessage(&publish.Message)
		if err != nil {
			c.opt.Log.Errorf("onMessage topic=%s payload=%x err=%v", publish.Message.Topic, publish.Message.Payload, err)
			_ = c.disconnect(err)
			return
		}
	}

	if publish.Message.QOS == packet.QOSAtLeastOnce {
		puback := packet.NewPuback()
		puback.ID = publish.ID
		err := c.send(puback)
		if err != nil {
			// TODO retry send()
			_ = c.disconnect(err)
			return
		}
	}

	if publish.Message.QOS == packet.QOSExactlyOnce {
		panic("code error qos=2 not implemented")
	}
}

func (c *Client) onPuback(id packet.ID) {
	c.flowPublish.Lock()
	defer c.flowPublish.Unlock()
	if c.flowPublish.fu == nil {
		c.opt.Log.Errorf("unexpected PUBACK id=%d", id)
		return
	}
	if c.flowPublish.id != id {
		// given no concurrent publish flow of this code, PUBACK for unexpected id is severe error
		_ = c.disconnect(errors.Errorf("PUBACK id=%d expected=%d", id, c.flowPublish.id))
		return
	}
	c.flowPublish.fu.Complete(id)
}

func (c *Client) send(pkt packet.Generic) error {
	if cc := c.clientConn(true); cc != nil {
		return cc.send(pkt)
	}
	return ErrClientClosing
}

func (c *Client) worker() {
	stopch := c.alive.StopChan()
	for {
		cc := c.clientConn(true)
		if cc == nil {
			return
		}
		select {
		case <-cc.alive.WaitChan():

		case <-stopch:
			_ = cc.die(ErrClientClosing)
			return
		}

		c.opt.Log.Debugf("wait ReconnectDelay=%v", c.opt.ReconnectDelay)
		select {
		case <-time.After(c.opt.ReconnectDelay):

		case <-stopch:
			_ = cc.die(ErrClientClosing)
			return
		}
	}
}

// Single client connection. `transport.Conn` with CONNECT, SUBSCRIBE and pings.
// Differences from upstream 256dpi/gomqtt/client.Client:
// - observe connected and subscribed events via futures
// - no mutex, state is set once at creation, except transport.Conn which requires blocking Dial
// - subscribe once right after connect
type clientConn struct {
	alive  *alive.Alive
	closed uint32
	confu  *future.Future
	conn   atomic.Value // transport.Conn
	opt    ClientOptions
	pingat *atomic_clock.Clock // timestamp of last outgoing control packet
	pongat *atomic_clock.Clock // timestamp of last incoming control packet
	subfu  *future.Future
	subpkt *packet.Subscribe
}

func newClientConn(opt ClientOptions, subpkt *packet.Subscribe) *clientConn {
	cc := &clientConn{
		alive:  alive.NewAlive(), // TODO link to parent Client.alive
		confu:  future.New(),
		opt:    opt,
		pingat: atomic_clock.New(),
		pongat: atomic_clock.New(),
		subfu:  future.New(),
		subpkt: subpkt,
	}
	cc.alive.Add(1)
	go cc.connect()
	return cc
}

func (cc *clientConn) die(e error) error {
	if e == nil {
		e = ErrClientClosing
	}
	if !atomic.CompareAndSwapUint32(&cc.closed, 0, 1) {
		return e
	}
	cc.alive.Stop()
	cc.confu.Cancel(e)
	cc.subfu.Cancel(e)
	if conn := cc.getConn(); conn != nil {
		_ = conn.Close()
	}
	return e
}

func (cc *clientConn) getConn() transport.Conn {
	if x := cc.conn.Load(); x != nil {
		return x.(transport.Conn)
	}
	return nil
}

// dial, send CONNECT, wait CONNACK, start pinger and reader
func (cc *clientConn) connect() {
	defer cc.alive.Done()

	conn, err := cc.opt.dialer.Dial(cc.opt.BrokerURL)
	if err != nil {
		_ = cc.die(errors.Annotatef(err, "connect: dial broker=%s", cc.opt.BrokerURL))
		return
	}
	cc.conn.Store(conn)
	if err = cc.send(cc.opt.conpkt); err != nil {
		return
	}

	{ // expect CONNACK
		conn.SetReadTimeout(cc.opt.NetworkTimeout)
		pkt, err := conn.Receive()
		if err != nil {
			err = errors.Annotate(err, "connect: expect CONNACK")
			_ = cc.die(err)
			return
		}
		connack, ok := pkt.(*packet.Connack)
		if !ok {
			err = errors.Annotatef(client.ErrClientExpectedConnack, "connect: server error pkt=%s", PacketString(pkt))
			_ = cc.die(err)
			return
		}
		cc.opt.Log.Debugf("CONNACK=%s", connack.String())
		// return connection denied error and close connection if not accepted
		if connack.ReturnCode != packet.ConnectionAccepted {
			err = errors.Annotate(client.ErrClientConnectionDenied, connack.ReturnCode.String())
			_ = cc.die(err)
			return
		}
		cc.confu.Complete(true)
		conn.SetReadTimeout(0)
	}

	if !cc.alive.Add(3) {
		_ = cc.die(context.Canceled)
		return
	}
	cc.pongat.SetNow()
	go cc.pinger()
	go cc.reader()
	go cc.subscriber()
}

func (cc *clientConn) onSuback(suback *packet.Suback) {
	if suback.ID != cc.subpkt.ID {
		err := errors.Annotatef(client.ErrFailedSubscription, "SUBACK.id=%d != SUBSCRIBE.id=%d", suback.ID, cc.subpkt.ID)
		_ = cc.die(err)
		return
	}
	for _, code := range suback.ReturnCodes {
		if code == packet.QOSFailure {
			_ = cc.die(client.ErrFailedSubscription)
			return
		}
	}
	cc.subfu.Complete(true)
}

// Sends ping packets to keep the connection alive.
// PINGREQ is only sent if Keepalive-NetworkTimeout has passed since last command.
func (cc *clientConn) pinger() {
	defer cc.alive.Done()
	if cc.opt.KeepaliveSec == 0 {
		return
	}

	// [MQTT-3.1.2-24] basically says control packets must arrive at most KeepaliveSec*1.5 apart.
	keepalive := keepaliveAndHalf(cc.opt.KeepaliveSec)
	// Try to send PINGREQ as late as possible to keep network traffic to minimum while respecting possible network issues.
	interval := keepalive - cc.opt.NetworkTimeout
	stopch := cc.alive.StopChan()
	for cc.alive.IsRunning() {
		now := atomic_clock.Now()
		window := now.Sub(cc.pingat)
		sincePong := now.Sub(cc.pongat)
		cc.opt.Log.Debugf("pinger keepalive=%v interval=%v window=%v sincePong=%v", keepalive, interval, window, sincePong)

		if window > 0 && window < interval {
			cc.opt.Log.Debugf("pinger sleep=%v", interval-window)
			select {
			case <-time.After(interval - window):
				continue

			case <-stopch:
				return
			}
		} else if window >= interval {
			if err := cc.send(packet.NewPingreq()); err != nil {
				return
			}
		}

		if sincePong > keepalive {
			_ = cc.die(client.ErrClientMissingPong)
			return
		}
	}
}

func (cc *clientConn) reader() {
	defer cc.alive.Done()

	conn := cc.getConn()
	// assert(conn!=nil)
	for {
		// get next packet from connection
		pkt, err := conn.Receive()
		if !cc.alive.IsRunning() {
			return
		}
		switch err {
		case nil: // success path

		case io.EOF: // server closed connection
			cc.opt.Log.Errorf("server closed connection")
			_ = cc.die(nil)
			return

		default:
			_ = cc.die(errors.Annotate(err, "receive"))
			return
		}
		cc.opt.Log.Debugf("received=%s", PacketString(pkt))

		switch pt := pkt.(type) {
		case *packet.Connack:
			_ = cc.die(errors.Errorf("server error duplicate CONNACK pkt=%s", PacketString(pkt)))
			return

		case *packet.Pingresp:
			cc.pongat.SetNow()

		case *packet.Suback:
			cc.onSuback(pt)

		default:
			cc.opt.onpacket(cc, pkt)
		}
	}
}

func (cc *clientConn) send(p packet.Generic) error {
	if cc == nil {
		return client.ErrClientNotConnected
	}
	conn := cc.getConn()
	if err := conn.Send(p, false); err != nil {
		err = errors.Annotatef(err, "send %s", p.Type().String())
		return cc.die(err)
	}
	cc.pingat.SetNow()
	cc.opt.Log.Debugf("sent %s", PacketString(p))
	return nil
}

func (cc *clientConn) subscriber() {
	defer cc.alive.Done()
	if cc.subpkt == nil {
		cc.subfu.Complete(true)
		return
	}

	if err := cc.send(cc.subpkt); err != nil {
		return
	}

	if cc.subfu.Wait(cc.opt.NetworkTimeout) == future.ErrTimeout {
		_ = cc.die(errors.Timeoutf("subscribe"))
	}
}

// Returns, in this order:
// - ErrClosing if clientConn is in final invalid state
// - nil if connected and subscribed within context limit
// - context.Canceled if context canceled/expired before successful connection
func (cc *clientConn) waitReady(ctx context.Context) error {
	if cc == nil {
		return ErrClientClosing
	}

	// TODO wait minimum time as opposed to mean pollInterval/2
	// select {
	// case <-cc.confu.CompletedChan():
	// case <-cc.confu.CanceledChan():
	// case <-ctx.Done():
	// }

	pollInterval := 500 * time.Millisecond
	if deadline, ok := ctx.Deadline(); ok {
		if timeout := -time.Since(deadline); timeout > 0 && timeout < pollInterval {
			pollInterval = timeout
		} else if timeout <= 0 {
			pollInterval = 1
		}
	}

	donech := ctx.Done()
	for {
		if !cc.alive.IsRunning() {
			return ErrClientClosing
		}
		_ = cc.confu.Wait(pollInterval)
		_ = cc.subfu.Wait(pollInterval)
		connected, _ := cc.confu.Result().(bool)
		subscribed, _ := cc.subfu.Result().(bool)
		if connected && subscribed {
			return nil
		}

		select {
		case <-time.After(pollInterval):

		case <-donech:
			return context.Canceled
		}
	}
}
