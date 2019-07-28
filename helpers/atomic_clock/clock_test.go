package atomic_clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const delta = 100 * time.Millisecond

func TestApi(t *testing.T) {
	c := New(0)

	c.Set(0)
	assert.True(t, c.IsZero())

	c.SetNow()
	assert.True(t, Since(c) < delta)

	assert.InDelta(t, c.UnixNano(), Source(), float64(delta))

	assert.InDelta(t, 0, Now().Sub(c), float64(delta))
}

func TestIfZero(t *testing.T) {
	c := New(0)
	src := Source()

	assert.True(t, c.IsZero())
	c.SetIfZero(src)
	assert.False(t, c.IsZero())

	c.SetIfZero(1)
	assert.NotEqual(t, 1, c.UnixNano())
}

func TestTimeCompatible(t *testing.T) {
	c := Now()
	tim := time.Now()

	assert.InDelta(t, tim.UnixNano(), c.UnixNano(), float64(delta))
	assert.InDelta(t, tim.Unix(), c.Unix(), 1)

	c.Set(tim.UnixNano())
	assert.Equal(t, tim.UnixNano(), c.UnixNano())
}
