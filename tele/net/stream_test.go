package telenet_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/tele"
	telenet "github.com/temoto/vender/tele/net"
)

func TestDecoderSuccess(t *testing.T) {
	t.Parallel()
	type genFunc = func(t testing.TB) tele.Packet
	// id := func(p tele.Packet) genFunc { return func(t testing.TB) tele.Packet { return p } }
	cases := []struct {
		name string
		hex  string
		gen  genFunc
	}{
		{"hello+ffs", "7602001c082a180b20808db790fb97affc1528cbffeb8bc6f5ebbcda01800201ffffffff",
			func(t testing.TB) tele.Packet {
				p, err := telenet.NewPacketHello(42, 1583222800532752000, "", 11, []byte("9ac98d2ef90c"))
				require.NoError(t, err)
				return p
			}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			genpkt := c.gen(t)

			b, err := telenet.FrameMarshal(&genpkt)
			t.Logf("frame=%x", b)
			require.NoError(t, err)
			assert.Equal(t, c.hex[:len(b)*2], hex.EncodeToString(b))

			b, err = hex.DecodeString(c.hex)
			require.NoError(t, err, "code error in test")
			dec := telenet.Decoder{}
			dec.Attach(bufio.NewReader(bytes.NewReader(b)), 1024)
			var p *tele.Packet
			p, err = dec.Read()
			require.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, genpkt.String(), p.String())
		})
	}
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
		{"76020001", "readfull: unexpected EOF"},
		{"760200110000000000000000000000000000000000", "frameLen=17 exceeds max=16"},
		{"7602000f0000000000000000000000000000", "readfull: unexpected EOF"},
		{"7602000100", "illegal tag 0"},
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
