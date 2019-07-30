package atomic_float

import (
	"math"
	"sync/atomic"
)

type F32 uint32

func (f *F32) Load() float32 {
	return math.Float32frombits(atomic.LoadUint32((*uint32)(f)))
}

func (f *F32) Store(new float32) {
	atomic.StoreUint32((*uint32)(f), math.Float32bits(new))
}

func (f *F32) Add(delta float32) float32 {
tryAgain:
	oldbits := atomic.LoadUint32((*uint32)(f))
	old := math.Float32frombits(oldbits)
	new := old + delta
	newbits := math.Float32bits(new)
	if atomic.CompareAndSwapUint32((*uint32)(f), oldbits, newbits) {
		return new
	}
	goto tryAgain // can't inline for loop
}
