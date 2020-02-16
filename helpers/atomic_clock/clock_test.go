package atomic_clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const delta = 100 * time.Millisecond

func TestApi(t *testing.T) {
	c := New()

	c.Set(0)
	assert.True(t, c.IsZero())

	c.SetNow()
	assert.True(t, Since(c) < delta)

	assert.InDelta(t, 0, Now().Sub(c), float64(delta))
}

func TestIfZero(t *testing.T) {
	c := New()
	src := Source()

	assert.True(t, c.IsZero())
	c.SetIfZero(src)
	assert.False(t, c.IsZero())

	c.SetIfZero(1)
	assert.NotEqual(t, 1, c.get())
}
