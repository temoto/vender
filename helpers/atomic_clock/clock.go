// Package atomic_clock is convenient API around atomic int64 system clock.
// Use for time accounting. Do not use where time zone matters.
package atomic_clock

import (
	"sync/atomic"
	"time"
)

type Clock struct{ v int64 }

func source() int64 { return time.Now().UnixNano() }

func (c *Clock) get() int64         { return atomic.LoadInt64(&c.v) }
func (c *Clock) set(new int64)      { atomic.StoreInt64(&c.v, new) }
func (c *Clock) cas(old, new int64) { atomic.CompareAndSwapInt64(&c.v, old, new) }

func (c *Clock) IsZero() bool { return c.get() == 0 }

func (c *Clock) Set(new int64)       { c.set(new) }
func (c *Clock) SetIfZero(new int64) { c.cas(0, new) }
func (c *Clock) SetNow()             { c.set(source()) }
func (c *Clock) SetNowIfZero()       { c.cas(0, source()) }

func (c *Clock) Sub(begin *Clock) time.Duration { return time.Duration(c.get() - begin.get()) }

func (c *Clock) Unix() int64     { return c.get() / int64(time.Second) }
func (c *Clock) UnixNano() int64 { return c.get() }

func New(v int64) *Clock { return &Clock{v: v} }
func Now() *Clock        { return New(source()) }

func Since(begin *Clock) time.Duration { return time.Duration(source() - begin.get()) }
func Source() int64                    { return source() }
