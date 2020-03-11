package telenet_test

import (
	"context"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/tele"
	telenet "github.com/temoto/vender/tele/net"
)

func TestClientNominal(t *testing.T) {
	t.Parallel()
	log := log2.NewTest(t, log2.LDebug)
	// log := log2.NewStderr(log2.LDebug) // useful with panics
	log.SetFlags(log2.Lmicroseconds | log2.Lshortfile)
	servopt := telenet.ConnOptions{
		Log:            log.Clone(log2.LDebug),
		NetworkTimeout: time.Second,
		ReadLimit:      telenet.DefaultReadLimit,
	}
	servopt.Log.SetPrefix("server: ")
	server := mockServerStream(t, 1, servopt, func(conn telenet.Conn) {
		defer conn.Close()
		servopt.Log.Printf("accept remote=%s", conn.RemoteAddr())
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
		defer cancel()

		testConnExpectHello(t, ctx, conn, true)

		testConnExpectState(t, ctx, conn, tele.State_Boot, true)

		p3, err := conn.Receive(ctx)
		require.NoError(t, err)
		log.Printf("server: receive p=%s err=%v", p3, err)

		t.Logf("server: stat=%s", conn.Stat())
	})
	defer server.Close()
	cliopt := testClientOptions(log, "tcp://"+server.Addr().String())
	t.Logf("client: stream=%s", cliopt.StreamURL)
	cli, err := telenet.NewClient(cliopt)
	require.NoError(t, err)
	assert.NoError(t, cli.Send(context.Background(), &tele.Packet{State: tele.State_Boot}))
	assert.NoError(t, cli.Send(context.Background(), &tele.Packet{Telemetry: &tele.Telemetry{Error: &tele.Telemetry_Error{Message: "something bad happened"}}}))
	cli.Close()
	t.Logf("client: stat=%s", cli.Stat())
}

// Custom server will accept only one connection and disconnect after 1 second network timeout.
// To succeed, client must send keepalive packets.
func TestClientKeepalive(t *testing.T) {
	t.Parallel()
	// log := log2.NewTest(t, log2.LDebug)
	log := log2.NewStderr(log2.LDebug) // useful with panics
	log.SetFlags(log2.Lmicroseconds | log2.Lshortfile)
	servopt := telenet.ConnOptions{
		GetSecret:      func(string, *tele.Frame) ([]byte, error) { return []byte("secret123"), nil },
		Log:            log,
		NetworkTimeout: time.Second,
		ReadLimit:      telenet.DefaultReadLimit,
	}
	server := mockServerStream(t, 1, servopt, func(conn telenet.Conn) {
		defer conn.Close()
		log.Printf("server: accept remote=%s", conn.RemoteAddr())
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(2*time.Second))
		defer cancel()

		testConnExpectHello(t, ctx, conn, true)

		testConnExpectState(t, ctx, conn, tele.State_Boot, true)
		for i := 1; i <= 5; i++ {
			p, err := conn.Receive(ctx)
			require.NoError(t, err)
			log.Printf("server: receive p=%s err=%v", p, err)
			if p.State == tele.State_Nominal {
				i = 5 // exit after ack
			} else {
				assert.True(t, p.Ping)
			}
		}

		t.Logf("server: stat=%s", conn.Stat())
	})
	defer server.Close()
	cliopt := testClientOptions(log, "tcp://"+server.Addr().String())
	cliopt.Keepalive = 400 * time.Millisecond
	t.Logf("client: stream=%s", cliopt.StreamURL)
	cli, err := telenet.NewClient(cliopt)
	require.NoError(t, err)
	assert.NoError(t, cli.Send(context.Background(), &tele.Packet{State: tele.State_Boot}))
	time.Sleep(1500 * time.Millisecond)
	assert.NoError(t, cli.Send(context.Background(), &tele.Packet{State: tele.State_Nominal}))
	cli.Close()
	t.Logf("client: stat=%s", cli.Stat())
}

func testClientOptions(log *log2.Log, stream string) *telenet.ClientOptions {
	opt := &telenet.ClientOptions{
		BuildVersion: "0.20200304.0",
		StreamURL:    stream,
		VmId:         tele.VMID(rand.Uint32()),
	}
	opt.Log = log.Clone(log2.LDebug)
	opt.Log.SetPrefix("client: ")
	opt.NetworkTimeout = time.Second
	opt.GetSecret = func(string, *tele.Frame) ([]byte, error) { return []byte("password"), nil }
	opt.OnPacket = func(conn telenet.Conn, p *tele.Packet) error {
		from, _ := conn.ID()
		log.Printf("client: onpacket from=%s p=%s", from, p)
		return nil
	}
	opt.ReadLimit = telenet.DefaultReadLimit
	return opt
}

func testConnExpectHello(t testing.TB, ctx context.Context, conn telenet.Conn, ack bool) *tele.Packet {
	p, err := conn.Receive(ctx)
	require.NoError(t, err)
	t.Logf("server: receive p=%s err=%v", p, err)
	assert.True(t, p.Hello)
	timestamp := time.Now().UnixNano()
	assert.InDelta(t, timestamp, p.Time, float64(5*time.Second))
	conn.SetID(p.AuthId, tele.VMID(p.VmId))
	return p
}

func testConnExpectState(t testing.TB, ctx context.Context, conn telenet.Conn, expect tele.State, ack bool) *tele.Packet {
	p, err := conn.Receive(ctx)
	require.NoError(t, err)
	t.Logf("server: receive p=%s err=%v", p, err)
	assert.Equal(t, expect, p.State)
	return p
}

func mockServerStream(t testing.TB, count int, opt telenet.ConnOptions, fun func(telenet.Conn)) net.Listener {
	ll, err := net.Listen("tcp", "")
	require.NoError(t, err)
	go func() {
		defer ll.Close()
		for i := 0; i < count; i++ {
			netConn, err := ll.Accept()
			require.NoError(t, err)
			t.Logf("server: accept")
			go fun(telenet.NewStreamConn(netConn, opt))
		}
	}()
	return ll
}
