package telenet

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/tele"
)

const (
	DefaultNetworkTimeout = 30 * time.Second
	DefaultReadLimit      = 16 << 10
)

var ErrClosing = fmt.Errorf("closing")

type Conn interface {
	Close() error
	Closed() bool
	ID() (string, tele.VMID)
	Options() *ConnOptions
	Receive(context.Context) (*tele.Packet, error)
	RemoteAddr() net.Addr
	Send(context.Context, *tele.Packet) error
	SetID(authid string, vmid tele.VMID)
	SinceLastRecv() time.Duration
	Stat() *SessionStat
	String() string

	die(error) error
}

type ConnOptions struct {
	Log       *log2.Log
	GetSecret func(authid string) []byte
	TLS       *tls.Config

	NetworkTimeout time.Duration
	RetryDelay     time.Duration
	OnPacket       func(string, *tele.Packet) error
	ReadLimit      uint32
}

func DialContext(ctx context.Context, dialer net.Dialer, url string, opt ConnOptions) (Conn, error) {
	if dialer.Timeout == 0 {
		dialer.Timeout = opt.NetworkTimeout
	}
	if deadline, _ := ctx.Deadline(); !deadline.IsZero() {
		if timeout := time.Until(deadline); timeout > 0 && timeout < dialer.Timeout {
			dialer.Timeout = timeout
		} else if timeout < 0 {
			return nil, context.Canceled
		}
	}

	scheme, hostport, err := parseURI(url)
	if err != nil {
		return nil, err
	}

	var conn net.Conn
	switch scheme {
	case "tcp":
		conn, err = dialer.DialContext(ctx, "tcp", hostport)

	case "tls":
		config := opt.TLS
		if config.ServerName == "" {
			config = config.Clone()
			if config.ServerName, _, err = net.SplitHostPort(hostport); err != nil {
				return nil, err
			}
		}
		conn, err = dialer.DialContext(ctx, "tcp", hostport)
		if err == nil {
			conn = tls.Client(conn, config)
		}

	default:
		err = fmt.Errorf("unknown protocol=%s", scheme)
	}
	if err != nil {
		return nil, err
	}
	return NewStreamConn(conn, opt), nil
}
