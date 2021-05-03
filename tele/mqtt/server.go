package mqtt_dpi256

// Vender telemetry specific MQTT server.
// Part of public API for external usage.

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/256dpi/gomqtt/broker"
	"github.com/256dpi/gomqtt/client/future"
	"github.com/256dpi/gomqtt/packet"
	"github.com/256dpi/gomqtt/topic"
	"github.com/256dpi/gomqtt/transport"
	"github.com/juju/errors"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

const defaultReadLimit = 1 << 20

var (
	ErrSameClient    = fmt.Errorf("clientid overtake")
	ErrClosing       = fmt.Errorf("server is closing")
	ErrNoSubscribers = fmt.Errorf("no subscribers")
)

type ServerOptions struct {
	Log       *log2.Log
	ForceSubs []packet.Subscription
	OnClose   CloseFunc // valid client connection lost
	OnConnect ConnectFunc
	OnPublish MessageFunc
}

type CloseFunc = func(clientID string, clean bool, e error)
type ConnectFunc = func(context.Context, *BackendOptions, *packet.Connect) (bool, error)
type MessageFunc = func(context.Context, *packet.Message, *future.Future) error

// Server.subs is prefix tree of pattern -> []{client, qos}
type subscription struct {
	pattern string
	client  string
	qos     packet.QOS
}

type Server struct { //nolint:maligned
	sync.RWMutex

	alive    *alive.Alive
	backends struct {
		sync.RWMutex
		m map[string]*backend
	}
	ctx       context.Context
	forceSubs []packet.Subscription
	listens   map[string]*transport.NetServer
	log       *log2.Log
	nextid    uint32 // atomic packet.ID
	onClose   CloseFunc
	onConnect ConnectFunc
	onPublish MessageFunc
	retain    *topic.Tree // *packet.Message
	subs      *topic.Tree // *subscription
}

func NewServer(opt ServerOptions) *Server {
	if opt.OnPublish == nil {
		panic("code error mqtt.ServerOptions.OnPublish is mandatory")
	}
	s := &Server{
		alive:  alive.NewAlive(),
		retain: topic.NewStandardTree(),
		subs:   topic.NewStandardTree(),
	}
	s.backends.m = make(map[string]*backend)
	s.forceSubs = opt.ForceSubs
	s.log = opt.Log
	s.onConnect = defaultAuthDenyAll
	if opt.OnConnect != nil {
		s.onConnect = opt.OnConnect
	}
	s.onClose = opt.OnClose
	s.onPublish = opt.OnPublish
	return s
}

func (s *Server) Addrs() []string {
	s.RLock()
	defer s.RUnlock()
	addrs := make([]string, 0, len(s.listens))
	for _, l := range s.listens {
		addrs = append(addrs, l.Addr().String())
	}
	return addrs
}

func (s *Server) Close() error {
	// serialize well with acceptLoop
	s.alive.Stop()
	errs := make([]error, 0)
	helpers.WithLock(s, func() {
		for key, ns := range s.listens {
			if err := ns.Close(); err != nil {
				errs = append(errs, err)
			}
			delete(s.listens, key)
		}
	})
	helpers.WithLock(s.backends.RLocker(), func() {
		for _, b := range s.backends.m {
			switch err := b.die(nil); err {
			case nil, ErrClosing, io.EOF:

			default:
				errs = append(errs, err)
			}
		}
	})
	s.alive.Wait()
	return helpers.FoldErrors(errs)
}

func (s *Server) Listen(ctx context.Context, lopts []*BackendOptions) error {
	s.Lock()
	defer s.Unlock()

	s.ctx = ctx
	s.listens = make(map[string]*transport.NetServer, len(lopts))

	errs := make([]error, 0)
	for _, opt := range lopts {
		s.log.Debugf("listen url=%s timeout=%v", opt.URL, opt.NetworkTimeout)
		if opt.AckTimeout == 0 {
			opt.AckTimeout = 2 * opt.NetworkTimeout
		}
		if opt.ReadLimit == 0 {
			opt.ReadLimit = defaultReadLimit
		}

		ns, err := s.listen(opt)
		if err != nil {
			err = errors.Annotatef(err, "mqtt listen url=%s", opt.URL)
			errs = append(errs, err)
			continue
		}
		if !s.alive.Add(1) {
			errs = append(errs, errors.Errorf("Listen after Close"))
			break
		}
		s.listens[opt.URL] = ns
		go s.acceptLoop(ns, opt)
	}
	return helpers.FoldErrors(errs)
}

func (s *Server) NextID() packet.ID {
	u32 := atomic.AddUint32(&s.nextid, 1)
	return packet.ID(u32 % (1 << 16))
}

func (s *Server) Publish(ctx context.Context, msg *packet.Message) error {
	s.log.Debugf("Server.Publish msg=%s", MessageString(msg))
	id := s.NextID()

	if msg.Retain {
		if len(msg.Payload) != 0 {
			s.retain.Set(msg.Topic, msg.Copy())
		} else {
			s.retain.Empty(msg.Topic)
		}
	}

	var _a [8]*subscription
	subs := _a[:0]
	uniq := make(map[string]struct{}) // deduplicate subscriptions
	for _, x := range s.subs.Match(msg.Topic) {
		xsub := x.(*subscription)
		if _, ok := uniq[xsub.client]; !ok {
			uniq[xsub.client] = struct{}{}
			subs = append(subs, xsub)
		}
	}
	n := len(subs)
	s.log.Debugf("Server.Publish len(subs)=%d", n)
	if n == 0 {
		return ErrNoSubscribers
	}

	errch := make(chan error, n)
	successCount := uint32(0)
	wg := sync.WaitGroup{}
	helpers.WithLock(s.backends.RLocker(), func() {
		for _, sub := range subs {
			b, ok := s.backends.m[sub.client]
			if !ok {
				continue
			}
			wg.Add(1)
			bmsg := msg.Copy()
			bmsg.QOS = sub.qos
			go func() {
				defer wg.Done()
				if err := b.Publish(ctx, id, bmsg); err != nil {
					errch <- err
				} else {
					atomic.AddUint32(&successCount, 1)
				}
			}()
		}
	})
	wg.Wait()
	close(errch)
	atomic.LoadUint32(&successCount)
	return helpers.FoldErrChan(errch)
}

func (s *Server) Retain() []*packet.Message {
	xs := s.retain.All()
	if len(xs) == 0 {
		return nil
	}
	ms := make([]*packet.Message, len(xs))
	for i, x := range xs {
		ms[i] = x.(*packet.Message)
	}
	return ms
}

func (s *Server) listen(opt *BackendOptions) (*transport.NetServer, error) {
	u, err := url.ParseRequestURI(opt.URL)
	if err != nil {
		return nil, errors.Annotate(err, "parse url")
	}

	var ns *transport.NetServer
	switch u.Scheme {
	case "tls":
		if ns, err = transport.CreateSecureNetServer(u.Host, opt.TLS); err != nil {
			return nil, errors.Annotate(err, "CreateSecureNetServer")
		}

	case "tcp", "unix":
		listen, err := net.Listen(u.Scheme, u.Host)
		if err != nil {
			return nil, errors.Annotatef(err, "net.Listen network=%s address=%s", u.Scheme, u.Host)
		}
		ns = transport.NewNetServer(listen)
	}
	if ns == nil {
		return nil, errors.Errorf("unsupported listen url=%s", opt.URL)
	}
	return ns, nil
}

func (s *Server) acceptLoop(ns *transport.NetServer, opt *BackendOptions) {
	defer s.alive.Done() // one alive subtask for each listener
	for {
		conn, err := ns.Accept()
		if !s.alive.IsRunning() {
			return
		}
		if err != nil {
			err = errors.Annotatef(err, "accept listen=%s", opt.URL)
			// TODO for extra cheese, this error must propagate to s.Close() return value
			s.log.Error(err)
			s.alive.Stop()
			return
		}

		if !s.alive.Add(1) { // and one alive subtask for each connection
			_ = conn.Close()
			return
		}
		go s.processConn(conn, opt)
	}
}

func (s *Server) onAccept(ctx context.Context, conn transport.Conn, opt *BackendOptions) (*backend, error) {
	var pkt packet.Generic
	var err error
	addr := addrString(conn.RemoteAddr())
	defer errors.DeferredAnnotatef(&err, "addr=%s", addr)
	// Receive first packet without backend
	pkt, err = conn.Receive()
	if err != nil {
		return nil, errors.Trace(err)
	}

	pktConnect, ok := pkt.(*packet.Connect)
	if !ok {
		err = broker.ErrUnexpectedPacket
		return nil, errors.Trace(err)
	}

	connack := packet.NewConnack()
	connack.SessionPresent = false

	// Server MAY allow a Client to supply a ClientId that has a length of zero bytes,
	// however if it does so the Server MUST treat this as a special case and assign a unique ClientId to that Client
	// if pktConnect.ClientID == "" && pktConnect.CleanSession { clientID = randomSaltPlusConnPort() }
	if pktConnect.ClientID == "" {
		connack.ReturnCode = packet.IdentifierRejected
		_ = conn.Send(connack, false)
		err = errors.Annotatef(broker.ErrNotAuthorized, "invalid clientid=%s", pktConnect.ClientID)
		return nil, errors.Trace(err)
	}

	ok, err = s.onConnect(ctx, opt, pktConnect)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !ok {
		connack.ReturnCode = packet.NotAuthorized
		_ = conn.Send(connack, false)
		err = broker.ErrNotAuthorized
		return nil, errors.Trace(err)
	}
	willString := "-"
	if pktConnect.Will != nil {
		willString = pktConnect.Will.String()
	}
	s.log.Debugf("mqtt CONNECT addr=%s client=%s username=%s keepalive=%d will=%s",
		addr, pktConnect.ClientID, pktConnect.Username, pktConnect.KeepAlive, willString)

	connack.ReturnCode = packet.ConnectionAccepted
	requestedKeepAlive := time.Duration(pktConnect.KeepAlive) * time.Second
	if requestedKeepAlive == 0 || requestedKeepAlive > opt.NetworkTimeout {
		requestedKeepAlive = opt.NetworkTimeout
	}
	conn.SetReadTimeout(requestedKeepAlive + time.Duration(requestedKeepAlive/2)*time.Second)
	err = conn.Send(connack, false)
	if err != nil {
		return nil, errors.Trace(err)
	}

	b := newBackend(ctx, conn, opt, s.log, pktConnect)
	return b, nil
}

func defaultAuthDenyAll(ctx context.Context, opt *BackendOptions, pkt *packet.Connect) (bool, error) {
	return false, fmt.Errorf("default connect callback is deny-all, please supply ServerOptions.OnConnect")
}

func (s *Server) onSubscribe(b *backend, pkt *packet.Subscribe) error {
	// The payload of a SUBSCRIBE packet MUST contain at least one Topic Filter / QoS pair.
	// A SUBSCRIBE packet with no payload is a protocol violation [MQTT-3.8.3-3].
	if len(pkt.Subscriptions) == 0 {
		return b.die(fmt.Errorf("subscribe request with empty sub list"))
	}
	suback := packet.NewSuback()
	suback.ID = pkt.ID
	suback.ReturnCodes = make([]packet.QOS, 0, len(pkt.Subscriptions))
	s.subscribe(b, pkt.Subscriptions, suback)
	err := b.Send(suback)
	err = errors.Annotate(err, "onSubscribe")
	return err
}

func (s *Server) processConn(conn transport.Conn, opt *BackendOptions) {
	defer s.alive.Done()

	addrNew := addrString(conn.RemoteAddr())
	conn.SetMaxWriteDelay(0)
	conn.SetReadLimit(opt.ReadLimit)
	conn.SetReadTimeout(opt.NetworkTimeout)
	b, err := s.onAccept(s.ctx, conn, opt)
	if err != nil {
		s.log.Infof("mqtt onAccept addr=%s err=%v", addrNew, err)
		_ = conn.Close()
		return
	}

	helpers.WithLock(&s.backends, func() {
		// close existing client with same id
		if ex, ok := s.backends.m[b.id]; ok {
			addrEx := addrString(ex.RemoteAddr())
			addrNew := addrString(b.getConn().RemoteAddr())
			s.log.Infof("mqtt client overtake id=%s ex=%s new=%s", b.id, addrEx, addrNew)
			_ = ex.die(ErrSameClient)
		}
		s.backends.m[b.id] = b
	})

	s.subscribe(b, s.forceSubs, nil)

	// receive loop
	wg := sync.WaitGroup{}
	for {
		var pkt packet.Generic
		pkt, err = b.Receive()
		if !b.alive.IsRunning() || !s.alive.IsRunning() {
			_ = b.die(ErrClosing)
			break
		}
		if err != nil {
			break
		}
		wg.Add(1)
		go s.processPacket(b, pkt, &wg)
	}
	wg.Wait()

	graceTimeout := b.opt.NetworkTimeout
	_ = b.acks.Await(graceTimeout)
	b.acks.Clear()
	b.alive.WaitTasks()

	// mandatory cleanup on backend closed
	closeErr := b.die(ErrClosing)
	will, clean := b.getWill()
	helpers.WithLock(&s.backends, func() {
		if ex := s.backends.m[b.id]; b == ex {
			s.log.Debugf("mqtt id=%s clean=%t will=%v", b.id, clean, will)
			delete(s.backends.m, b.id)
		}
		for _, value := range s.subs.All() {
			if sub := value.(*subscription); sub.client == b.id {
				s.subs.Remove(sub.pattern, value)
			}
		}
	})
	if !clean && will != nil {
		_ = s.Publish(s.ctx, will)
	}
	if s.onClose != nil {
		s.onClose(b.id, clean, closeErr)
	}
}

// on each incoming packet after connect handshake
func (s *Server) processPacket(b *backend, pkt packet.Generic, finally interface{ Done() }) {
	defer finally.Done()
	err := helpers.WithLockError(&s.backends, func() error {
		ex := s.backends.m[b.id]
		if b != ex {
			s.log.Errorf("mqtt processPacket ignore from detached id=%s pkt=%s", b.id, pkt.String())
			_ = b.die(ErrSameClient)
			return ErrSameClient
		}
		return nil
	})
	if err != nil {
		return
	}

typeSwitch:
	switch pt := pkt.(type) {
	case *packet.Pingreq:
		err = b.Send(packet.NewPingresp())

	case *packet.Publish:
		ack := future.New()
		err = s.onPublish(b.ctx, &pt.Message, ack)
		if err != nil {
			s.log.Errorf("mqtt onPublish msg=%s err=%v", pt.Message.String(), err)
			break typeSwitch
		}

		switch pt.Message.QOS {
		case packet.QOSAtMostOnce:
		case packet.QOSAtLeastOnce:
			switch ack.Wait(0) {
			case nil: // explicit ack
				pktPuback := packet.NewPuback()
				pktPuback.ID = pt.ID
				err = b.Send(pktPuback)

			case future.ErrCanceled: // explicit nack
				err = fmt.Errorf("publish rejected client=%s id=%d topic=%s message=%x", b.id, pt.ID, pt.Message.Topic, pt.Message.Payload)

			case future.ErrTimeout: // onPublish callback did not complete/cancel future
			}

		default:
			err = fmt.Errorf("qos %d is not supported", pt.Message.QOS)
		}

	case *packet.Puback:
		err = b.FulfillAck(pt.ID)

	case *packet.Subscribe:
		err = s.onSubscribe(b, pt)
		if err != nil {
			s.log.Errorf("mqtt onSubscribe err=%v", err)
		}

	case *packet.Pubrec, *packet.Pubrel, *packet.Pubcomp:
		err = fmt.Errorf("qos2 not supported")

	case *packet.Disconnect:
		b.onDisconnect()
		_ = b.die(nil)
		b.alive.Wait()
		return

	default:
		err = fmt.Errorf("code error packet is not handled pkt=%s", pkt.String())
	}
	if err != nil {
		_ = b.die(err)
	}
}

func (s *Server) subscribe(b *backend, subs []packet.Subscription, pktSubAck *packet.Suback) {
	for _, sub := range subs {
		pattern := sub.Topic
		pattern = strings.ReplaceAll(pattern, "%c", b.id)
		pattern = strings.ReplaceAll(pattern, "%u", b.username)
		sub2 := &subscription{
			pattern: pattern,
			client:  b.id,
			qos:     sub.QOS,
		}
		if sub2.qos > packet.QOSAtLeastOnce {
			sub2.qos = packet.QOSAtLeastOnce
		}
		s.subs.Add(pattern, sub2)
		if pktSubAck != nil {
			pktSubAck.ReturnCodes = append(pktSubAck.ReturnCodes, sub2.qos)
		}

		values := s.retain.Search(pattern)
		for _, v := range values {
			pid := s.NextID()
			msg := v.(*packet.Message)
			go func() {
				_ = b.Publish(s.ctx, pid, msg)
			}()
		}
	}
}

func addrString(a net.Addr) string {
	if a == nil {
		return ""
	}
	return a.String()
}
