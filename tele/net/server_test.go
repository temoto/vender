package telenet_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/tele"
	telenet "github.com/temoto/vender/tele/net"
)

func TestServerStreamNominal(t *testing.T) {
	t.Parallel()
	// log := log2.NewTest(t, log2.LDebug)
	log := log2.NewStderr(log2.LDebug) // useful with panics
	log.SetFlags(log2.Lmicroseconds | log2.Lshortfile)
	servopt := telenet.ServerOptions{
		Log: log,
		// OnAuth: func(conn telenet.Conn, p *tele.Packet) error {
		// 	log.Printf("server: auth conn=%s p=%s", conn, p)
		// 	return
		// },
		OnClose: func(authid string, vmid tele.VMID, e error) {
			log.Printf("server: id=%s vmid=%d disconnected", authid, vmid)
		},
		OnPacket: func(conn telenet.Conn, p *tele.Packet) error {
			return nil
		},
	}
	server, cli := testPairStream(t, &servopt, &telenet.ListenOptions{NetworkTimeout: time.Second}, nil)
	time.Sleep(time.Second)
	cli.Close()
	t.Logf("client: stat=%s", cli.Stat().String())
	t.Logf("server: stat=%s", server.Stat().String())
}

func testServerStream(t testing.TB, servopt *telenet.ServerOptions, lopt *telenet.ListenOptions) (*telenet.Server, string) {
	if lopt.StreamURL == "" {
		lopt.StreamURL = "tcp://"
	}
	s := telenet.NewServer(*servopt)
	require.NoError(t, s.Listen(context.Background(), []telenet.ListenOptions{*lopt}))
	addrs := s.Addrs()
	require.Equal(t, 1, len(addrs))
	return s, addrs[0]
}

func testPairStream(t testing.TB, servopt *telenet.ServerOptions, lopt *telenet.ListenOptions, cliopt *telenet.ClientOptions) (*telenet.Server, *telenet.Client) {
	s, addr := testServerStream(t, servopt, lopt)
	if cliopt == nil {
		cliopt = testClientOptions(nil, "")
	}
	cliopt.Log = servopt.Log
	if cliopt.OnPacket == nil {
		cliopt.OnPacket = func(from string, p *tele.Packet) error {
			cliopt.Log.Printf("client: onpacket from=%s p=%s", from, p)
			return nil
		}
	}
	cliopt.StreamURL = "tcp://" + addr // TODO scheme from lopt
	cliopt.VmId = tele.VMID(rand.Uint32())
	c, err := telenet.NewClient(cliopt)
	require.NoError(t, err)
	return s, c
}
