package slim

import (
	"context"
	"crypto/tls"
	fmt "fmt"
	"net"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/tele"
)

var ErrSameClient = fmt.Errorf("id overtake")

// Slim server.
type Server struct {
	alive *alive.Alive
	conns struct {
		sync.RWMutex
		m map[ID]Conn
	}
	listens struct {
		sync.RWMutex
		m map[string]net.Listener
	}
	log       *log2.Log
	onAuth    PayloadFunc
	onClose   CloseFunc
	onPayload PayloadFunc
	stat      SessionStat
}

type ServerOptions struct {
	// TODO parent Alive

	Log       *log2.Log
	OnAuth    PayloadFunc
	OnClose   CloseFunc // valid client connection lost
	OnPayload PayloadFunc
}

type ListenOptions struct {
	// TODO PacketURL string
	StreamURL      string
	TLS            *tls.Config
	NetworkTimeout time.Duration
	ReadLimit      uint16
}

func NewServer(opt ServerOptions) *Server {
	s := &Server{
		alive:     alive.NewAlive(),
		log:       opt.Log,
		onClose:   opt.OnClose,
		onPayload: opt.OnPayload,
	}
	s.conns.m = make(map[ID]Conn)
	s.listens.m = make(map[string]net.Listener)
	s.onAuth = defaultAuthDenyAll
	if opt.OnAuth != nil {
		s.onAuth = opt.OnAuth
	}
	return s
}

func (s *Server) Addrs() []string {
	s.listens.RLock()
	defer s.listens.RUnlock()
	addrs := make([]string, 0, len(s.listens.m))
	for _, l := range s.listens.m {
		addrs = append(addrs, l.Addr().String())
	}
	return addrs
}

func (s *Server) Listen(ctx context.Context, opts []ListenOptions) error {
	s.listens.Lock()
	defer s.listens.Unlock()

	for _, ll := range s.listens.m {
		ll.Close()
	}

	if !s.alive.Add(len(opts)) {
		return errors.Errorf("Listen after Close")
	}
	errs := make([]error, 0)
	for _, opt := range opts {
		s.log.Debugf("listen url=%s timeout=%v", opt.StreamURL, opt.NetworkTimeout)
		if opt.ReadLimit == 0 {
			opt.ReadLimit = DefaultReadLimit
		}
		if err := s.listenStream(opt); err != nil {
			s.alive.Done()
			err = errors.Annotatef(err, "listenStream %s", opt.StreamURL)
			errs = append(errs, err)
			continue
		}
	}
	return helpers.FoldErrors(errs)
}

func (s *Server) Stat() *SessionStat { return &s.stat }

func (s *Server) ConnOptions(lo *ListenOptions) ConnOptions {
	return ConnOptions{
		Log:            s.log,
		NetworkTimeout: lo.NetworkTimeout,
		ReadLimit:      lo.ReadLimit,
		TLS:            lo.TLS,
	}
}

func (s *Server) listenStream(opt ListenOptions) error {
	scheme, hostport, err := parseURI(opt.StreamURL)
	if err != nil {
		return errors.Annotate(err, "parse url")
	}

	var ll net.Listener
	switch scheme {
	case "tls":
		if ll, err = tls.Listen("tcp", hostport, opt.TLS); err != nil {
			return errors.Annotate(err, "tls.Listen")
		}

	case "tcp", "unix":
		ll, err = net.Listen(scheme, hostport)
		if err != nil {
			return errors.Annotatef(err, "net.Listen network=%s address=%s", scheme, hostport)
		}
	}
	if ll == nil {
		return errors.Errorf("unsupported listen url=%s", opt.StreamURL)
	}

	s.listens.m[opt.StreamURL] = ll
	go s.acceptLoop(ll, opt)
	return nil
}

func (s *Server) acceptLoop(ll net.Listener, opt ListenOptions) {
	defer s.alive.Done() // one alive subtask for each listener
	for {
		netConn, err := ll.Accept()
		if !s.alive.IsRunning() {
			return
		}
		if err != nil {
			err = errors.Annotatef(err, "accept listen=%s", addrString(ll.Addr()))
			// TODO for extra cheese, this error must propagate to s.Close() return value
			s.log.Error(err)
			s.alive.Stop()
			return
		}

		if !s.alive.Add(1) { // and one alive subtask for each connection
			_ = netConn.Close()
			return
		}
		conn, err := NewStreamConn(netConn, s.ConnOptions(&opt))
		if err != nil {
			s.alive.Done()
			s.log.Error(err)
			// TODO exit on fatal error
			continue
		}
		go s.processConn(conn)
	}
}

func defaultAuthDenyAll(conn Conn, payload []byte) error {
	return fmt.Errorf("default-deny-all")
}

func (s *Server) processConn(conn Conn) {
	defer s.alive.Done()

	<-conn.Done()

	// mandatory cleanup on connection closed
	authid := conn.ID()
	closeErr := conn.die(ErrClosing)
	helpers.WithLock(&s.conns, func() {
		if ex := s.conns.m[authid]; conn == ex {
			delete(s.conns.m, authid)
		}
	})
	if s.onClose != nil {
		s.onClose(authid, closeErr)
	}
}

func (s *Server) onHandshake(conn Conn, p *tele.Packet) error {
	var err error
	var hello *Frame
	addr := addrString(conn.RemoteAddr())
	defer errors.DeferredAnnotatef(&err, "addr=%s", addr)

	hello, err = conn.receive(context.Background())
	if err != nil {
		return errors.Trace(err)
	}

	// Handshake was successful.
	// conn.SetID(hello.OpaqueID)
	// s.log.Debugf("HELLO addr=%s id=%v", addr, hello.OpaqueID)
	// if err = conn.Send(context.Background(), response); err != nil {
	// 	return errors.Trace(err)
	// }

	helpers.WithLock(&s.conns, func() {
		// close existing client with same id
		if ex, ok := s.conns.m[hello.OpaqueID]; ok {
			addrEx := addrString(ex.RemoteAddr())
			addrNew := addrString(conn.RemoteAddr())
			s.log.Infof("client overtake id=%s ex=%s new=%s", hello.OpaqueID, addrEx, addrNew)
			_ = ex.die(ErrSameClient)
		}
		s.conns.m[hello.OpaqueID] = conn
	})

	return nil
}

// on each incoming packet only after successful handshake
func (s *Server) processPayload(conn Conn, f *Frame) {
	err := helpers.WithLockError(&s.conns, func() error {
		id := conn.ID()
		if ex := s.conns.m[id]; conn != ex {
			s.log.Infof("PLEASE REPORT BUG processPayload ignore from detached id=%v f=%s", id, f)
			_ = conn.die(ErrSameClient)
			return ErrSameClient
		}
		return nil
	})
	if err != nil {
		return
	}

	if err = s.onPayload(conn, f.Payload); err != nil {
		s.log.Errorf("onPayload f=%s err=%v", f, err)
		_ = conn.die(err)
	}
}
