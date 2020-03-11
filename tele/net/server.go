package telenet

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
	tele_config "github.com/temoto/vender/tele/config"
)

var ErrSameClient = fmt.Errorf("vmid overtake")

// Vender telemetry server, venderctl side.
// Used in venderctl and in testing the client.
type Server struct {
	alive *alive.Alive
	conns struct {
		sync.RWMutex
		m map[string]Conn
	}
	listens struct {
		sync.RWMutex
		m map[string]net.Listener
	}
	log      *log2.Log
	onAuth   PacketFunc
	onClose  CloseFunc
	onPacket PacketFunc
	stat     SessionStat
}

type ServerOptions struct {
	// TODO parent Alive

	Log      *log2.Log
	OnAuth   PacketFunc
	OnClose  CloseFunc // valid client connection lost
	OnPacket PacketFunc
}

type ListenOptions struct {
	// TODO PacketURL string
	StreamURL      string
	TLS            *tls.Config
	NetworkTimeout time.Duration
	ReadLimit      uint32
}

type CloseFunc = func(id string, vmid tele.VMID, e error)
type PacketFunc = func(Conn, *tele.Packet) error

func NewServer(opt ServerOptions) *Server {
	s := &Server{
		alive:    alive.NewAlive(),
		log:      opt.Log,
		onClose:  opt.OnClose,
		onPacket: opt.OnPacket,
	}
	s.conns.m = make(map[string]Conn)
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

func (s *Server) authorize(conn Conn, p *tele.Packet) error {
	wantRole := tele_config.RoleInvalid
	switch {
	case p.Hello && p.AuthId == fmt.Sprintf("vm%d", p.VmId):
		wantRole = tele_config.RoleVender

	case p.Hello:
		wantRole = tele_config.RoleAll

	case p.State != tele.State_Invalid || p.Telemetry != nil:
		wantRole = tele_config.RoleVender

	case p.Command != nil || p.Response != nil:
		wantRole = tele_config.RoleControl
	}

	if wantRole == tele_config.RoleInvalid {
		return fmt.Errorf("code error authorize cant infer required role")
	}
	// TODO
	return nil
}

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
		conn, err := ll.Accept()
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
			_ = conn.Close()
			return
		}
		go s.processConn(NewStreamConn(conn, s.ConnOptions(&opt)))
	}
}

func (s *Server) handshake(conn Conn) error {
	var err error
	var p *tele.Packet
	addr := addrString(conn.RemoteAddr())
	defer errors.DeferredAnnotatef(&err, "addr=%s", addr)

	p, err = conn.Receive(context.Background())
	if err != nil {
		return errors.Trace(err)
	}

	if !p.Hello {
		err = tele.ErrUnexpectedPacket
		return errors.Trace(err)
	}
	if p.AuthId == "" {
		p.AuthId = fmt.Sprintf("vm%d", p.VmId)
	}

	reply := &tele.Packet{
		Hello: true,
		Time:  time.Now().UnixNano(),
		// TODO SessionId:  s.newsid(),
	}
	if err = s.authorize(conn, p); err != nil {
		reply.Error = "auth:" + err.Error()
		_ = conn.Send(context.Background(), reply)
		err = tele.ErrNotAuthorized
		return errors.Trace(err)
	}

	// Handshake was successful.
	conn.SetID(p.AuthId, tele.VMID(p.VmId))
	s.log.Debugf("HELLO addr=%s id=%s time=%d",
		addr, p.AuthId, p.Time)
	if err = conn.Send(context.Background(), reply); err != nil {
		return errors.Trace(err)
	}

	helpers.WithLock(&s.conns, func() {
		// close existing client with same id
		if ex, ok := s.conns.m[p.AuthId]; ok {
			addrEx := addrString(ex.RemoteAddr())
			addrNew := addrString(conn.RemoteAddr())
			s.log.Infof("client overtake id=%s ex=%s new=%s", p.AuthId, addrEx, addrNew)
			_ = ex.die(ErrSameClient)
		}
		s.conns.m[p.AuthId] = conn
	})
	return nil
}

func defaultAuthDenyAll(conn Conn, p *tele.Packet) error {
	return fmt.Errorf("default-deny-all")
}

func (s *Server) processConn(conn Conn) {
	defer s.alive.Done()

	if err := s.handshake(conn); err != nil {
		addrNew := addrString(conn.RemoteAddr())
		s.log.Infof("onAccept addr=%s err=%v", addrNew, err)
		_ = conn.die(err)
		return
	}

	// receive loop
	for {
		p, err := conn.Receive(context.Background())
		if !s.alive.IsRunning() {
			_ = conn.die(ErrClosing)
			break
		}
		if err != nil {
			break
		}
		s.processPacket(conn, p)
	}

	// conn.alive.WaitTasks()

	// mandatory cleanup on connection closed
	authid, vmid := conn.ID()
	closeErr := conn.die(ErrClosing)
	helpers.WithLock(&s.conns, func() {
		if ex := s.conns.m[authid]; conn == ex {
			delete(s.conns.m, authid)
		}
	})
	if s.onClose != nil {
		s.onClose(authid, vmid, closeErr)
	}
}

// on each incoming packet only after successful handshake
func (s *Server) processPacket(conn Conn, p *tele.Packet) {
	err := helpers.WithLockError(&s.conns, func() error {
		authid, _ := conn.ID()
		if ex := s.conns.m[authid]; conn != ex {
			s.log.Infof("PLEASE REPORT processPacket ignore from detached id=%s p=%s", authid, p.String())
			_ = conn.die(ErrSameClient)
			return ErrSameClient
		}
		return nil
	})
	if err != nil {
		return
	}

	if err = s.onPacket(conn, p); err != nil {
		s.log.Errorf("onPacket p=%s err=%v", p.String(), err)
		_ = conn.die(err)
	}
}
