package mqtt

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/256dpi/gomqtt/packet"
	"github.com/256dpi/gomqtt/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/log2"
)

func TestClient(t *testing.T) {
	t.Parallel()
	const timeout = 5 * time.Second

	type tenv struct {
		addr         string
		sync1, sync2 chan struct{}
		alive        *alive.Alive
		opts         ClientOptions
	}
	cases := []struct {
		name   string
		client func(t testing.TB, env *tenv)
		server func(t testing.TB, env *tenv, b *transport.NetConn)
	}{
		{"connect", func(t testing.TB, env *tenv) {
			mc, err := NewClient(env.opts)
			require.NoError(t, err)
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(timeout))
			defer cancel()
			require.NoError(t, mc.WaitReady(ctx))
		}, func(t testing.TB, env *tenv, b *transport.NetConn) {
			defer env.alive.Done()
			pkt, err := b.Receive()
			require.NoError(t, err)
			assert.Equal(t, `<Connect ClientID="" KeepAlive=0 Username="" Password="" CleanSession=true Will=nil Version=4>`, pkt.String())
			connack := packet.NewConnack()
			connack.ReturnCode = packet.ConnectionAccepted
			require.NoError(t, b.Send(connack, false))
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			env := &tenv{
				alive: alive.NewAlive(),
				sync1: make(chan struct{}),
				sync2: make(chan struct{}),
			}
			ln, err := net.Listen("tcp", "127.0.0.1:")
			require.NoError(t, err)
			env.addr = ln.Addr().String()
			env.opts.BrokerURL = fmt.Sprintf("tcp://%s", env.addr)
			env.opts.OnMessage = func(m *packet.Message) error {
				t.Log(m.String())
				return nil
			}
			env.opts.Log = log2.NewStderr(log2.LDebug)
			env.opts.NetworkTimeout = timeout
			env.alive.Add(1)
			go func() {
				defer env.alive.Done()
				for {
					conn, err := ln.Accept()
					if !env.alive.Add(1) {
						t.Log("env.alive stopped")
						return
					}
					require.NoError(t, err)
					require.NoError(t, conn.SetDeadline(time.Now().Add(timeout)))
					c.server(t, env, transport.NewNetConn(conn))
				}
			}()
			c.client(t, env)
			env.alive.Wait()
		})
	}
}
