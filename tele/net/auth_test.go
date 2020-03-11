package telenet

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/tele"
)

func TestAuth1(t *testing.T) {
	t.Parallel()
	type P = tele.Packet
	cases := []struct { //nolint:maligned
		dhex   string
		secret string
		expect uint64
	}{
		{"516bc9c0138765ff8f3f", "9d014afdc6a17816", 0xf200902cd2bc7b01},
	}
	for _, c := range cases {
		c := c
		t.Run(fmt.Sprintf("%016x", c.expect), func(t *testing.T) {
			data, err := hex.DecodeString(c.dhex)
			require.NoError(t, err)
			auth, err := Auth1(data, []byte(c.secret))
			require.NoError(t, err)
			assert.Equal(t, c.expect, auth)
		})
	}
}
