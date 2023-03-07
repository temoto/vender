// Package atomic_clock is convenient API around atomic int64 monotonic clock.
// Use for time accounting. Do not use where actual time value matters.
package atomic_clock

import (
	"sync/atomic"
	"time"
)

var epoch = time.Now()

type Clock struct{ v int64 }

func New() *Clock { return &Clock{v: 0} }
func Now() *Clock { return &Clock{v: source()} }

func Since(begin *Clock) time.Duration { return time.Duration(source() - begin.get()) }
func Source() int64                    { return source() }

func (c *Clock) IsZero() bool { return c.get() == 0 }

func (c *Clock) Set(new int64)       { c.set(new) }
func (c *Clock) SetIfZero(new int64) { c.cas(0, new) }
func (c *Clock) SetNow()             { c.set(source()) }
func (c *Clock) SetNowIfZero()       { c.cas(0, source()) }

func (c *Clock) Sub(begin *Clock) time.Duration { return time.Duration(c.get() - begin.get()) }

func source() int64 { return int64(time.Since(epoch)) }

func (c *Clock) get() int64         { return atomic.LoadInt64(&c.v) }
func (c *Clock) set(new int64)      { atomic.StoreInt64(&c.v, new) }
func (c *Clock) cas(old, new int64) { atomic.CompareAndSwapInt64(&c.v, old, new) }
