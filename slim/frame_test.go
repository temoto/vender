package slim_test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/slim"
)

// func TestFrameEncode(t *testing.T) {
// 	t.Parallel()
// 	rnd := helpers.RandUnix()
// 	gen := func(r *rand.Rand) *Frame {
// 		p := &tele.Packet{Time: r.Int63()}
// 		f := slim.NewFrame(uint16(r.Uint32()), p)
// 		f.Sack = r.Uint32()
// 		f.Acks = r.Uint32()
// 		return f
// 	}
// 	_ = rnd
// 	_ = gen
// }

func TestFrameMarshal(t *testing.T) {
	type F = slim.Frame // shortcut
	cases := []struct {
		f      F
		expect string
	}{
		{F{Seq: 1}, "73020007000100"},
		{F{Seq: 12345, Payload: []byte("1")}, "7302000830390031"},
		{F{Seq: 47452, Session: 0x87654321feedfade}, "7302000fb95c0187654321feedfade"},
		{F{Flags: slim.V2FlagSig, Seq: 37654, Payload: []byte("example")}, "7302002e9316046578616d706c652794694dbf56a96d4c6a028fddb8783b0fd75d31d076731bae79e5d5c865bde5"},
	}
	for _, c := range cases {
		t.Run(c.f.String(), func(t *testing.T) {
			c.f.GetSecret = testGetSecretFIXME
			b, err := c.f.Marshal()
			require.NoError(t, err)
			assert.Equal(t, c.expect, hex.EncodeToString(b))
		})
	}
}

func TestFrameSize(t *testing.T) {
	type F = slim.Frame // shortcut
	cases := []struct {
		f      F
		expect int
	}{
		{F{}, 7},
		{F{Seq: 1}, 7},
		{F{Payload: []byte("1")}, 8},
		{F{Session: 123}, 15},
	}
	for _, c := range cases {
		t.Run(c.f.String(), func(t *testing.T) {
			c.f.ImplyFlags()
			assert.Equal(t, c.expect, c.f.Size())
		})
	}
}

// func TestFrameHmac(t *testing.T) {
// 	t.Parallel()
// 	cases := []struct { //nolint:maligned
// 		dhex   string
// 		secret string
// 		expect uint64
// 	}{
// 		{"516bc9c0138765ff8f3f", "9d014afdc6a17816", 0xf200902cd2bc7b01},
// 	}
// 	for _, c := range cases {
// 		c := c
// 		t.Run(fmt.Sprintf("%016x", c.expect), func(t *testing.T) {
// 			data, err := hex.DecodeString(c.dhex)
// 			require.NoError(t, err)
// 			auth, err := Auth1(data, []byte(c.secret))
// 			require.NoError(t, err)
// 			assert.Equal(t, c.expect, auth)
// 		})
// 	}
// }
