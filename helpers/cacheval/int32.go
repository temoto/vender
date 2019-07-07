// Atomic value with validity timeout.
// "modified" timestamp is updated after value, without consistency.
// Usage scenario examples: DNS resolve, sensor reading.
// All methods except `Init` are thread-safe.
package cacheval

import (
	"sync/atomic"
	"time"

	"github.com/temoto/vender/helpers/atomic_clock"
)

type Int32 struct {
	value   int32
	updated *atomic_clock.Clock
	valid   time.Duration
}

// Not thread-safe. `valid` duration cannot be changed later.
func (c *Int32) Init(valid time.Duration) {
	c.updated = atomic_clock.New(0)
	c.valid = valid
}

func (c *Int32) get(now int64) (int32, bool) {
	v := atomic.LoadInt32(&c.value)
	clock := atomic_clock.New(now)
	age := clock.Sub(c.updated)
	return v, age >= 0 && age <= c.valid
}

// Returns current (possibly stale) value. Fast and cheap.
func (c *Int32) Get() int32 { return atomic.LoadInt32(&c.value) }

// Returns current value and true if it's fresh. Costs current timestamp lookup.
func (c *Int32) GetFresh() (int32, bool) { return c.get(atomic_clock.Source()) }

// Always returns fresh value.
// If value is stale, runs `f()`.
// It is `f()` responsibility to update value with `Set()` method.
// No cache stampede guard.
// May return value from concurrent `GetOrUpdate` or `Set`.
// Costs current timestamp lookup.
func (c *Int32) GetOrUpdate(f func()) int32 {
	now := atomic_clock.Source()
	v, ok := c.get(now)
	if !ok {
		f()
		v = atomic.LoadInt32(&c.value)
	}
	return v
}

// Updates value and modified timestamp.
// Both value and timestamp are updated atomically, but not consistently with each other.
// Costs current timestamp lookup.
func (c *Int32) Set(new int32) {
	atomic.StoreInt32(&c.value, new)
	c.updated.SetNow()
}
