package telenet

import (
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
		p      tele.Packet
		secret string
		expect uint64
	}{
		{P{Seq: 123, Time: 1583220597483015936, VmId: 1, AuthId: ""}, "9d014afdc6a17816", 0xb6ef9d03b2cae03f},
	}
	for _, c := range cases {
		c := c
		t.Run(fmt.Sprintf("%016x", c.expect), func(t *testing.T) {
			auth, err := Auth1(&c.p, []byte(c.secret))
			require.NoError(t, err)
			assert.Equal(t, c.expect, auth)
		})
	}
}
