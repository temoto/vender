package atomic_clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestApi(t *testing.T) {
	c := Now()
	tim := time.Now()
	const delta = 100 * time.Millisecond

	assert.InDelta(t, tim.UnixNano(), c.UnixNano(), float64(delta))
	assert.InDelta(t, tim.Unix(), c.Unix(), 1)

	c.SetTime(tim)
	assert.Equal(t, tim.UnixNano(), c.UnixNano())
	assert.InDelta(t, tim.UnixNano(), c.Time().UnixNano(), float64(delta))

	c.SetNow()
	assert.True(t, Since(c) < delta)
}
