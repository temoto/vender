package helpers

import (
	"log"
	"sync/atomic"
	"time"

	"github.com/temoto/vender/helpers/atomic_clock"
)

// Limited exponential backoff for retry delays.
// Choose DelayAfter or DelayBefore whichever fits your code better.
// First delay is always 0.
// Update(false) or Failure() increases next delay by K.
type Backoff struct {
	next int64 // atomic align
	last atomic_clock.Clock

	Min time.Duration
	Max time.Duration
	K   float32
	Res time.Duration // delay resolution for nice logs, default=1ms
}

// Use scenario:
// for {
//   err := op()
//   time.Sleep(backoff.DelayAfter(err==nil))
// }
func (b *Backoff) DelayAfter(success bool) time.Duration {
	atomic.CompareAndSwapInt64(&b.next, 0, int64(b.Min))
	if success {
		b.Reset()
	} else {
		b.Failure()
	}
	return b.DelayBefore()
}

// Use scenario:
// for {
//   time.Sleep(backoff.DelayBefore())
//   err := op()
//   backoff.Update(err==nil)
// }
func (b *Backoff) DelayBefore() time.Duration {
	next := time.Duration(atomic.LoadInt64(&b.next))
	if next == 0 {
		return 0
	}
	delay := b.limit(time.Duration(atomic.LoadInt64(&b.next)))
	since := atomic_clock.Since(&b.last)
	log.Printf("backoff delay next=%s delay=%s since=%s", next, delay, since)
	if since >= delay {
		return 0
	}
	return b.round(delay - since)
}

// Increase next Delay()
func (b *Backoff) Failure() {
	next := time.Duration(atomic.LoadInt64(&b.next))
	next = time.Duration(float32(next) * b.K)
	next = b.limit(next)
	b.last.SetNow()
	atomic.StoreInt64(&b.next, int64(next))
}

func (b *Backoff) Reset() {
	b.last.SetNow()
	atomic.StoreInt64(&b.next, int64(b.Min))
}

func (b *Backoff) Update(success bool) {
	if success {
		b.Reset()
	} else {
		b.Failure()
	}
}

func (b *Backoff) limit(d time.Duration) time.Duration {
	if d < b.Min {
		d = b.Min
	}
	if d > b.Max {
		d = b.Max
	}
	return b.round(d)
}

func (b *Backoff) round(d time.Duration) time.Duration {
	res := b.Res
	if res == 0 {
		res = 1 * time.Millisecond
	}
	return d / res * res
}
