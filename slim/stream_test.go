package slim_test

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
	"github.com/temoto/vender/slim"
)

var testSecretFIXME = []byte("password")

func testGetSecretFIXME(_ slim.ID, _ *slim.Frame) ([]byte, error) {
	return append([]byte(nil), testSecretFIXME...), nil
}

func TestDecoderSuccess(t *testing.T) {
	t.Parallel()
	type F = slim.Frame
	cases := []struct {
		name string
		hex  string
		f    F
	}{
		{"hello+ffs", "7302000c007b0068656c6c6fffffffff",
			F{Seq: 123, Payload: []byte("hello")}},
		{"hello_signed+ffs", "7302002c007b0468656c6c6fd67df7766f57b7b84bf2fedd331d5ecba2721bef980bbcf1458f967175872a1effffffff",
			F{Flags: slim.V2FlagSig, Seq: 123, Payload: []byte("hello")}},
		{"session+zeros", "73020014007b01f1234567b765432168656c6c6f00000000",
			F{Seq: 123, Session: 0xf1234567b7654321, Payload: []byte("hello")}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			c.f.GetSecret = testGetSecretFIXME
			b, err := c.f.Marshal()
			t.Logf("marshal=%x", b)
			require.NoError(t, err)
			assert.Equal(t, c.hex[:len(b)*2], hex.EncodeToString(b))

			b, err = hex.DecodeString(c.hex)
			require.NoError(t, err, "code error in test")
			dec := slim.Decoder{GetSecret: testGetSecretFIXME}
			dec.Attach(bufio.NewReader(bytes.NewReader(b)), 1024)
			var fin *slim.Frame
			fin, err = dec.Read()
			require.NoError(t, err)
			require.NotNil(t, fin)
			assert.Equal(t, c.f.String(), fin.String())
		})
	}
}

func TestDecoderSequence(t *testing.T) {
	t.Parallel()
	const input = "73020007000108730200090002003132ffee"
	b, err := hex.DecodeString(input)
	require.NoError(t, err, "code error in test")
	dec := slim.Decoder{}
	dec.Attach(bufio.NewReader(bytes.NewReader(b)), 1024)
	var fin *slim.Frame
	fin, err = dec.Read()
	require.NoError(t, err)
	require.NotNil(t, fin)
	assert.Equal(t, "(seq=1 flags=k payload=(0))", fin.String())
	fin, err = dec.Read()
	require.NoError(t, err)
	require.NotNil(t, fin)
	assert.Equal(t, "(seq=2 flags= payload=(2)3132)", fin.String())
}

func TestDecoderError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		hex    string
		expect string
	}{
		{"", "EOF"},
		{"73", "header: unexpected EOF"},
		{"7302", "header: unexpected EOF"},
		{"730200", "header: unexpected EOF"},
		{"73020000", "header: unexpected EOF"},
		{"73020001aabb00", "length=1: frame is invalid"},
		{"ee020000ddee00", "wrong magic: frame is invalid"},
		{"ee020007ddee00", "wrong magic: frame is invalid"},
		{"73020008ccff00", "frame body: unexpected EOF"},
		{"7302001155660000000000000000000000000000000000", "frame length=17 exceeds max=16"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.hex, func(t *testing.T) {
			b, err := hex.DecodeString(c.hex)
			require.NoError(t, err, "code error in test")
			dec := slim.Decoder{}
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
		assert.Equal(t, "730200100001006869696d616c696365", hex.EncodeToString(buf))
		req := &slim.Frame{GetSecret: cliopt.GetSecret}
		require.NoError(t, req.Unmarshal(buf))
		ack := &slim.Frame{AckSeq: 1, Acks: 1, GetSecret: cliopt.GetSecret}
		ackb, err := ack.Marshal()
		require.NoError(t, err)
		require.NoError(t, helpers.WriteAll(nc2, ackb))
	}()
	conn1, err := slim.NewStreamConn(nc1, cliopt.ConnOptions)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, conn1.Send(ctx, []byte("hiimalice")))
	<-done2
}
