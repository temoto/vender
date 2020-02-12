package mqtt_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/256dpi/gomqtt/client/future"
	"github.com/256dpi/gomqtt/packet"
	"github.com/256dpi/gomqtt/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/tele/mqtt"
)

const testDefaultTimeout = 1000 * time.Millisecond

type tenv struct {
	t    testing.TB
	ctx  context.Context
	log  *log2.Log
	sopt *mqtt.ServerOptions
	s    *mqtt.Server
	addr string
	rand *rand.Rand
	clid string
	data interface{}
}

func TestServer(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*tenv)
		check func(*tenv)
	}{
		{name: "invalid-credentials", check: func(env *tenv) {
			conn := connDial(env)
			pktConnect := packet.NewConnect()
			pktConnect.CleanSession = false
			pktConnect.ClientID = "cli"
			pktConnect.Username = "unknown"
			require.NoError(env.t, conn.Send(pktConnect, false))
			pktConnack := connReceive(env, conn).(*packet.Connack)
			assert.False(env.t, pktConnack.SessionPresent)
			assert.Equal(env.t, packet.NotAuthorized, pktConnack.ReturnCode)
		}},
		{name: "accepted-clean", check: func(env *tenv) {
			conn := connDial(env)
			connConnect(env, conn, "", nil)
		}},
		{name: "sub-qos0", check: func(env *tenv) {
			conn := connDial(env)
			connConnect(env, conn, "", nil)
			connSubscribe(env, conn, []packet.Subscription{{Topic: "#", QOS: packet.QOSAtMostOnce}})
			msgout := packet.Message{Topic: "todo_random", QOS: packet.QOSAtMostOnce, Payload: []byte("todo_random")}
			connPublish(env, conn, msgout)
			pktPublish := connReceive(env, conn).(*packet.Publish)
			assert.Equal(env.t, msgout.Topic, pktPublish.Message.Topic)
			assert.Equal(env.t, msgout.Payload, pktPublish.Message.Payload)
		}},
		{name: "sub-qos1-pub-qos0", check: func(env *tenv) {
			conn := connDial(env)
			connConnect(env, conn, "", nil)
			connSubscribe(env, conn, []packet.Subscription{{Topic: "#", QOS: packet.QOSAtLeastOnce}})
			msgout := packet.Message{Topic: "todo_random", QOS: packet.QOSAtMostOnce, Payload: []byte("todo_random")}
			connPublish(env, conn, msgout)
			pktPublish := connReceive(env, conn).(*packet.Publish)
			assert.Equal(env.t, msgout.Topic, pktPublish.Message.Topic)
			assert.Equal(env.t, msgout.Payload, pktPublish.Message.Payload)
			require.Equal(env.t, packet.QOSAtLeastOnce, pktPublish.Message.QOS)
			connPuback(env, conn, pktPublish.ID)
			time.Sleep(testDefaultTimeout / 2)
		}},
		{name: "will", check: func(env *tenv) {
			conn := connDial(env)
			connConnect(env, conn, "", nil)
			connSubscribe(env, conn, []packet.Subscription{{Topic: "#", QOS: packet.QOSAtMostOnce}})

			connTrigger := connDial(env)
			will := &packet.Message{Topic: "todo_random", Payload: []byte("todo_random")}
			connConnect(env, connTrigger, "", will)
			require.NoError(env.t, connTrigger.Close())

			pktPublish := connReceive(env, conn).(*packet.Publish)
			assert.Equal(env.t, will.Topic, pktPublish.Message.Topic)
			assert.Equal(env.t, will.Payload, pktPublish.Message.Payload)
			require.Equal(env.t, packet.QOSAtMostOnce, pktPublish.Message.QOS)
		}},
		{name: "will-not-sent-after-disconnect", check: func(env *tenv) {
			conn := connDial(env)
			connConnect(env, conn, "", nil)
			connSubscribe(env, conn, []packet.Subscription{{Topic: "#", QOS: packet.QOSAtMostOnce}})

			connTrigger := connDial(env)
			will := &packet.Message{Topic: "todo_random", Payload: []byte("todo_random"), Retain: true}
			connConnect(env, connTrigger, "", will)
			connDisconnect(env, connTrigger)
			require.NoError(env.t, connTrigger.Close())

			require.Len(env.t, env.s.Retain(), 0)
		}},
		{name: "forced-sub", setup: func(env *tenv) {
			env.sopt = &mqtt.ServerOptions{
				ForceSubs: []packet.Subscription{
					{Topic: "%c/r/#", QOS: packet.QOSAtLeastOnce},
				},
			}
			testServerDefaultSetup(env)
		}, check: func(env *tenv) {
			conn := connDial(env)
			id := fmt.Sprintf("cli%d", env.rand.Int31())
			connConnect(env, conn, id, nil)
			// no explicit client subscribe

			topic := fmt.Sprintf("%s/r/yodawg", id)
			msgout := packet.Message{Topic: topic, QOS: packet.QOSAtMostOnce, Payload: []byte("todo_random")}
			sent := make(chan struct{})
			go func() {
				assert.NoError(t, env.s.Publish(env.ctx, &msgout))
				close(sent)
			}()
			pktPublish := connReceive(env, conn).(*packet.Publish)
			assert.Equal(env.t, msgout.Topic, pktPublish.Message.Topic)
			assert.Equal(env.t, msgout.Payload, pktPublish.Message.Payload)
			<-sent
			connDisconnect(env, conn)
		}},
		{name: "explicit-and-forced-sub-deliver-once", setup: func(env *tenv) {
			env.sopt = &mqtt.ServerOptions{
				ForceSubs: []packet.Subscription{
					{Topic: "%c/r/#", QOS: packet.QOSAtLeastOnce},
				},
			}
			testServerDefaultSetup(env)
		}, check: func(env *tenv) {
			conn := connDial(env)
			id := fmt.Sprintf("cli%d", env.rand.Int31())
			connConnect(env, conn, id, nil)
			connSubscribe(env, conn, []packet.Subscription{{Topic: "#", QOS: packet.QOSAtLeastOnce}})

			topic := fmt.Sprintf("%s/r/yodawg", id)
			msgout := packet.Message{Topic: topic, Payload: []byte("todo_random")}
			sent := make(chan struct{})
			go func() {
				assert.NoError(t, env.s.Publish(env.ctx, &msgout))
				close(sent)
			}()
			pktPublish := connReceive(env, conn).(*packet.Publish)
			assert.Equal(env.t, msgout.Topic, pktPublish.Message.Topic)
			assert.Equal(env.t, msgout.Payload, pktPublish.Message.Payload)
			connPuback(env, conn, pktPublish.ID)
			<-sent
			connDisconnect(env, conn)
		}},
		{name: "onclose-clean", setup: func(env *tenv) {
			env.clid = fmt.Sprintf("cli%d", env.rand.Int31())
			env.sopt = &mqtt.ServerOptions{
				OnClose: func(clientid string, clean bool, e error) {
					env.data = assert.Equal(env.t, env.clid, clientid) && assert.True(env.t, clean)
				},
			}
			testServerDefaultSetup(env)
		}, check: func(env *tenv) {
			conn := connDial(env)
			connConnect(env, conn, env.clid, nil)
			connDisconnect(env, conn)
			_, err := conn.Receive() // synchronization
			require.Error(env.t, err)
			assert.NoError(env.t, env.s.Close()) // synchronization
			called, _ := env.data.(bool)
			assert.True(env.t, called, "onclose was not called")
		}},
		{name: "onclose-abrupt", setup: func(env *tenv) {
			env.clid = fmt.Sprintf("cli%d", env.rand.Int31())
			env.sopt = &mqtt.ServerOptions{
				OnClose: func(clientid string, clean bool, e error) {
					env.data = assert.Equal(env.t, env.clid, clientid) && assert.False(env.t, clean)
				},
			}
			testServerDefaultSetup(env)
		}, check: func(env *tenv) {
			conn := connDial(env)
			connConnect(env, conn, env.clid, nil)
			require.NoError(env.t, conn.Close())
			assert.NoError(env.t, env.s.Close()) // synchronization
			called, _ := env.data.(bool)
			assert.True(env.t, called, "onclose was not called")
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			env := &tenv{
				t:    t,
				ctx:  context.Background(),
				log:  log2.NewTest(t, log2.LDebug),
				rand: helpers.RandUnix(),
			}
			if os.Getenv("vender_test_log_stderr") == "1" {
				env.log = log2.NewStderr(log2.LDebug) // useful with panics
			}
			env.log.SetFlags(log2.LTestFlags)
			if c.setup == nil {
				c.setup = testServerDefaultSetup
			}
			defer func() {
				assert.NoError(t, env.s.Close())
			}()
			c.setup(env)
			c.check(env)
		})
	}
}

func TestServerCloseListen(t *testing.T) {
	t.Parallel()

	s := mqtt.NewServer(mqtt.ServerOptions{OnPublish: func(ctx context.Context, msg *packet.Message, ack *future.Future) error {
		t.Error("unexpected call OnPublish")
		return nil
	}})
	require.NoError(t, s.Close())
	lopts := []*mqtt.BackendOptions{{URL: "tcp://localhost:"}}
	err := s.Listen(context.Background(), lopts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Listen after Close")
}

func newTestServer(env *tenv, opt mqtt.ServerOptions, lopts []*mqtt.BackendOptions) (*mqtt.Server, string) {
	if opt.Log == nil {
		opt.Log = env.log
	}
	s := mqtt.NewServer(opt)
	require.NoError(env.t, s.Listen(context.Background(), lopts))
	addrs := s.Addrs()
	require.Equal(env.t, len(lopts), len(addrs))
	firstAddr := ""
	if len(addrs) >= 1 {
		firstAddr = addrs[0]
	}
	return s, firstAddr
}

func testServerDefaultSetup(env *tenv) {
	sopt := mqtt.ServerOptions{
		OnConnect: authFromMap(map[string]string{"testuser": "testsecret"}),
		OnPublish: func(ctx context.Context, msg *packet.Message, ack *future.Future) error {
			env.log.Infof("OnPublish msg=%s", mqtt.MessageString(msg))
			return env.s.Publish(ctx, msg)
		},
	}
	if env.sopt != nil && env.sopt.ForceSubs != nil {
		sopt.ForceSubs = env.sopt.ForceSubs
	}
	if env.sopt != nil && env.sopt.OnClose != nil {
		sopt.OnClose = env.sopt.OnClose
	}
	// if env.sopt!=nil&&env.sopt.OnConnect!=nil{ sopt.OnConnect=env.sopt.OnConnect }
	// if env.sopt!=nil&&env.sopt.OnPublish!=nil{ sopt.OnPublish=env.sopt.OnPublish }
	lopts := []*mqtt.BackendOptions{
		{
			URL:            "tcp://localhost:",
			CtxData:        env,
			NetworkTimeout: testDefaultTimeout,
		}}
	env.s, env.addr = newTestServer(env, sopt, lopts)
}

func connDial(env *tenv) transport.Conn {
	addr := "tcp://" + env.addr
	c, err := transport.Dial(addr)
	require.NoError(env.t, err)
	env.log.Infof("testClient dial %s", addr)
	c.SetReadTimeout(testDefaultTimeout)
	return c
}

func connConnect(env *tenv, c transport.Conn, id string, will *packet.Message) {
	if id == "" {
		id = fmt.Sprintf("cli%d", env.rand.Int31())
	}
	pktConnect := packet.NewConnect()
	pktConnect.CleanSession = true
	pktConnect.ClientID = id
	pktConnect.Username = "testuser"
	pktConnect.Password = "testsecret"
	pktConnect.Will = will
	require.NoError(env.t, c.Send(pktConnect, false))
	env.log.Infof("testClient sent %s", mqtt.PacketString(pktConnect))
	pktConnack := connReceive(env, c).(*packet.Connack)
	assert.False(env.t, pktConnack.SessionPresent)
	assert.Equal(env.t, packet.ConnectionAccepted, pktConnack.ReturnCode)
}

func connPublish(env *tenv, c transport.Conn, msg packet.Message) {
	pktPublish := packet.NewPublish()
	pktPublish.ID = packet.ID(env.rand.Uint32() % (1 << 16))
	pktPublish.Message = msg
	require.NoError(env.t, c.Send(pktPublish, false))
	env.log.Infof("testClient sent %s", mqtt.PacketString(pktPublish))
	switch msg.QOS {
	case packet.QOSAtMostOnce:
		return

	case packet.QOSAtLeastOnce:
		pktPuback := connReceive(env, c).(*packet.Puback)
		assert.Equal(env.t, pktPublish.ID, pktPuback.ID)

	default:
		panic("code error qos=2 not supported")
	}
}

func connReceive(env *tenv, c transport.Conn) packet.Generic {
	pkt, err := c.Receive()
	if pkt == nil {
		env.log.Infof("testClient recv pkt=nil err=%v", err)
	} else {
		env.log.Infof("testClient recv pkt=%s err=%v", mqtt.PacketString(pkt), err)
	}
	require.NoError(env.t, err)
	return pkt
}

func connSubscribe(env *tenv, c transport.Conn, subs []packet.Subscription) {
	pktSubscribe := packet.NewSubscribe()
	pktSubscribe.ID = packet.ID(env.rand.Uint32() % (1 << 16))
	pktSubscribe.Subscriptions = subs
	require.NoError(env.t, c.Send(pktSubscribe, false))
	env.log.Infof("testClient sent %s", mqtt.PacketString(pktSubscribe))
	pktSuback := connReceive(env, c).(*packet.Suback)
	expect := make([]packet.QOS, 0, len(subs))
	for _, sub := range subs {
		expect = append(expect, sub.QOS)
	}
	assert.Equal(env.t, expect, pktSuback.ReturnCodes)
}

func connPuback(env *tenv, c transport.Conn, id packet.ID) {
	pkt := packet.NewPuback()
	pkt.ID = id
	require.NoError(env.t, c.Send(pkt, false))
	env.log.Infof("testClient sent %s", mqtt.PacketString(pkt))
}

func connDisconnect(env *tenv, c transport.Conn) {
	pkt := packet.NewDisconnect()
	require.NoError(env.t, c.Send(pkt, false))
	env.log.Infof("testClient sent %s", mqtt.PacketString(pkt))
}

func authFromMap(m map[string]string) mqtt.ConnectFunc {
	return func(ctx context.Context, opt *mqtt.BackendOptions, pkt *packet.Connect) (bool, error) {
		if secret, ok := m[pkt.Username]; ok {
			return pkt.Password == secret, nil
		}
		return false, nil
	}
}
