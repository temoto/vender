package mqtt_dpi256

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/256dpi/gomqtt/client/future"
	"github.com/256dpi/gomqtt/packet"
	"github.com/256dpi/gomqtt/transport"
	"github.com/juju/errors"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

type BackendOptions struct {
	CtxData interface{} // opaque not touched by mqtt package
	URL     string
	TLS     *tls.Config

	AckTimeout     time.Duration
	NetworkTimeout time.Duration // conn receive timeout
	ReadLimit      int64
}

// Server side connection state.
// Relatively thin transport.Conn wrapper.
type backend struct {
	alive    *alive.Alive
	acks     *future.Store
	conn     transport.Conn
	connmu   sync.RWMutex
	disco    uint32
	ctx      context.Context
	err      helpers.AtomicError
	id       string
	opt      *BackendOptions
	log      *log2.Log
	username string
	will     *packet.Message
	willmu   sync.Mutex
}

func newBackend(ctx context.Context, conn transport.Conn, opt *BackendOptions, log *log2.Log, pktConnect *packet.Connect) *backend {
	b := &backend{
		alive:    alive.NewAlive(),
		conn:     conn,
		ctx:      ctx,
		id:       pktConnect.ClientID,
		opt:      opt,
		log:      log,
		username: pktConnect.Username,
	}
	b.acks = future.NewStore()
	if pktConnect.Will != nil {
		b.will = pktConnect.Will.Copy()
	}
	return b
}

func (b *backend) Close() error {
	b.alive.Stop()
	b.alive.Wait()
	err, _ := b.err.Load()
	return err
}

func (b *backend) Options() BackendOptions { return *b.opt }

func (b *backend) ExpectAck(ctx context.Context, id packet.ID) *future.Future {
	f := future.New()
	if !b.alive.Add(1) {
		f.Cancel(ErrClosing)
		return f
	}
	go func() {
		defer b.alive.Done()
		if err := f.Wait(b.opt.AckTimeout); err == future.ErrTimeout {
			f.Cancel(err)
		}
		b.acks.Delete(id)
	}()

	if ex := b.acks.Get(id); ex != nil {
		err := errors.Errorf("CRITICAL ExpectAck overwriting id=%d", id)
		b.log.Error(err)
		ex.Cancel(err)
		f.Cancel(err)
		return f
	}
	b.acks.Put(id, f)
	return f
}

// High level publish flow with QOS.
// TODO qos1 ack-timeout retry
func (b *backend) Publish(ctx context.Context, id packet.ID, msg *packet.Message) error {
	if !b.alive.Add(1) {
		return ErrClosing
	}
	defer b.alive.Done()

	pub := packet.NewPublish()
	pub.ID = id
	pub.Message = *msg
	switch msg.QOS {
	case packet.QOSAtMostOnce:
		pub.ID = 0
		return b.Send(pub)

	case packet.QOSAtLeastOnce:
		if pub.ID == 0 {
			return errors.Errorf("backend.doPublish QOSAtLeastOnce requires non-zero packet.ID message=%s", pub.Message.String())
		}
		f := b.ExpectAck(ctx, pub.ID)
		// TODO retry
		if err := b.Send(pub); err != nil {
			f.Cancel(err)
		}
		err := f.Wait(b.opt.AckTimeout)
		if err == nil {
			return nil // success path
		} else if err == future.ErrCanceled {
			if err, _ = f.Result().(error); err == nil {
				err = errors.Errorf("code error ack future canceled with nil")
			}
		}
		err = errors.Annotatef(err, "expect puback id=%d", pub.ID)
		return b.die(err)

	default:
		panic("code error QOS > 1 is not supported")
	}
}

func (b *backend) Receive() (packet.Generic, error) {
	conn := b.getConn()
	if conn == nil {
		return nil, ErrClosing
	}
	pkt, err := conn.Receive()
	b.log.Debugf("mqtt recv addr=%s id=%s pkt=%s err=%v", addrString(conn.RemoteAddr()), b.id, PacketString(pkt), err)
	switch err {
	case nil:
		return pkt, nil

	case io.EOF: // remote properly closed connection
		_ = b.die(err)
		return nil, err

	default:
		if !b.alive.IsRunning() && isClosedConn(err) {
			// conn.Close was used to interrupt blocking Send/Receive
			return nil, ErrClosing
		}
		_ = b.die(err)
		return nil, err
	}
}

func (b *backend) Send(pkt packet.Generic) error {
	conn := b.getConn()
	if conn == nil {
		return ErrClosing
	}
	b.log.Debugf("mqtt send id=%s pkt=%s", b.id, PacketString(pkt))
	if err := b.conn.Send(pkt, false); err != nil {
		if !b.alive.IsRunning() && isClosedConn(err) {
			// conn.Close was used to interrupt blocking Send/Receive
			return ErrClosing
		}
		err = errors.Annotatef(err, "clientid=%s", b.id)
		return b.die(err)
	}
	return nil
}

// success counterpart to ExpectAck
func (b *backend) FulfillAck(id packet.ID) error {
	f := b.acks.Get(id)
	if f == nil {
		return fmt.Errorf("unexpected ack for packet id=%d", id)
	}
	if !f.Complete(nil) {
		return future.ErrCanceled
	}
	return nil
}

func (b *backend) RemoteAddr() net.Addr {
	if conn := b.getConn(); conn != nil {
		return conn.RemoteAddr()
	}
	return nil
}

func (b *backend) die(e error) error {
	err, found := b.err.StoreOnce(e)
	if found {
		return err
	}
	b.log.Debugf("mqtt die id=%s e=%v", b.id, e)
	b.alive.Stop()
	helpers.WithLock(&b.connmu, func() {
		if b.conn != nil {
			b.log.Debugf("mqtt die +close id=%s addr=%s", b.id, addrString(b.conn.RemoteAddr()))
			_ = b.conn.Close()
			b.conn = nil
		}
	})
	return err
}

func (b *backend) getConn() transport.Conn {
	b.connmu.RLock()
	c := b.conn
	b.connmu.RUnlock()
	return c
}

func (b *backend) getWill() (m *packet.Message, clean bool) {
	b.willmu.Lock()
	if b.will != nil {
		m = b.will.Copy()
	}
	b.willmu.Unlock()
	clean = atomic.LoadUint32(&b.disco) == 1
	return m, clean
}

func (b *backend) onDisconnect() {
	atomic.StoreUint32(&b.disco, 1)
	b.willmu.Lock()
	b.will = nil
	b.willmu.Unlock()
}

func isClosedConn(e error) bool {
	return e != nil && strings.HasSuffix(e.Error(), "use of closed network connection")
}
