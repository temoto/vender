package telenet_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/tele"
	telenet "github.com/temoto/vender/tele/net"
)

var testSecretFIXME = []byte("password")

func TestDecoderSuccess(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		hex  string
		p    tele.Packet
	}{
		{"hello+ffs", "7602002001107b2a0f08808db790fb97affc15100b800201b0897cf625c6b5f2ffffffff",
			telenet.NewPacketHello(1583222800532752000, "", 11)},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			fout := telenet.NewFrame(123, &c.p)
			b, err := telenet.FrameMarshal(fout, testSecretFIXME)
			t.Logf("fout=%x", b)
			require.NoError(t, err)
			assert.Equal(t, c.hex[:len(b)*2], hex.EncodeToString(b))

			b, err = hex.DecodeString(c.hex)
			require.NoError(t, err, "code error in test")
			dec := telenet.Decoder{
				GetSecret: func(*tele.Frame) ([]byte, error) { return testSecretFIXME, nil },
			}
			dec.Attach(bufio.NewReader(bytes.NewReader(b)), 1024)
			var fin *tele.Frame
			fin, err = dec.Read()
			require.NoError(t, err)
			require.NotNil(t, fin)
			assert.Equal(t, c.p.String(), fin.Packet.String())
		})
	}
}

func TestDecoderSequence(t *testing.T) {
	t.Parallel()
	const input = "7602000700080176020007001002"
	b, err := hex.DecodeString(input)
	require.NoError(t, err, "code error in test")
	dec := telenet.Decoder{}
	dec.Attach(bufio.NewReader(bytes.NewReader(b)), 1024)
	var fin *tele.Frame
	fin, err = dec.Read()
	require.NoError(t, err)
	require.NotNil(t, fin)
	assert.Equal(t, "sid:1 ", fin.String())
	fin, err = dec.Read()
	require.NoError(t, err)
	require.NotNil(t, fin)
	assert.Equal(t, "seq:2 ", fin.String())
}

func TestDecoderError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		hex    string
		expect string
	}{
		{"", "EOF"},
		{"76", "header: unexpected EOF"},
		{"7602", "header: unexpected EOF"},
		{"760200", "header: unexpected EOF"},
		{"76020000", "header: unexpected EOF"},
		{"7602000100", "frameLen=1 too small: frame is invalid"},
		{"ee02000500", "wrong magic: frame is invalid"},
		{"7602000600", "frame body: unexpected EOF"},
		{"760200110000000000000000000000000000000000", "frameLen=17 exceeds max=16"},
		{"760200060000", "illegal tag 0"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.hex, func(t *testing.T) {
			b, err := hex.DecodeString(c.hex)
			require.NoError(t, err, "code error in test")
			dec := telenet.Decoder{}
			dec.Attach(bufio.NewReader(bytes.NewReader(b)), 16)
			p, err := dec.Read()
			require.Error(t, err)
			assert.Nil(t, p)
			assert.Contains(t, err.Error(), c.expect)
		})
	}
}

func TestStreamConnSend(t *testing.T) {
	t.Parallel()
	// log := log2.NewTest(t, log2.LDebug)
	log := log2.NewStderr(log2.LDebug) // useful with panics
	log.SetFlags(log2.Lmicroseconds | log2.Lshortfile)
	cliopt := testClientOptions(log, "mem://")
	done2 := make(chan struct{})
	nc1, nc2 := net.Pipe()
	go func() {
		defer close(done2)
		buf := make([]byte, 16<<10)
		n, err := nc2.Read(buf)
		assert.NoError(t, err)
		buf = buf[:n]
		t.Logf("server: recv=%x", buf)
		assert.Equal(t, "7602000d0010012a0408015001", hex.EncodeToString(buf))
		req := &tele.Frame{}
		require.NoError(t, telenet.FrameUnmarshal(buf, req, func(f *tele.Frame) ([]byte, error) { return cliopt.GetSecret("", f) }))
		ack := &tele.Frame{Sack: 1, Acks: 1}
		ackb, err := telenet.FrameMarshal(ack, testSecretFIXME)
		require.NoError(t, err)
		require.NoError(t, helpers.WriteAll(nc2, ackb))
	}()
	conn1 := telenet.NewStreamConn(nc1, cliopt.ConnOptions)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, conn1.Send(ctx, &tele.Packet{Time: 1, Ping: true}))
	<-done2
}
