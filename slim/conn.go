package slim

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"time"

	"github.com/temoto/vender/log2"
)

const (
	DefaultNetworkTimeout = 30 * time.Second
	DefaultReadLimit      = math.MaxUint16
	DefaultRetryDelay     = 13 * time.Second
)

var ErrClosing = fmt.Errorf("closing")

type Conn interface {
	Close() error
	Closed() bool
	Done() <-chan struct{}
	ID() ID
	Options() *ConnOptions
	RemoteAddr() net.Addr
	Send(context.Context, []byte) error
	SetID(new ID)
	SinceLastRecv() time.Duration
	Stat() *SessionStat
	String() string

	die(error) error
	receive(context.Context) (*Frame, error)
}

type ConnOptions struct {
	Log       *log2.Log
	GetSecret GetSecretFunc
	TLS       *tls.Config

	AckDelay       time.Duration
	Keepalive      time.Duration
	NetworkTimeout time.Duration
	RetryDelay     time.Duration
	OnHandshake    PayloadFunc
	OnPayload      PayloadFunc
	ReadLimit      uint16
}

type CloseFunc = func(id ID, e error)
type PayloadFunc = func(Conn, []byte) error

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
	return NewStreamConn(conn, opt)
}
