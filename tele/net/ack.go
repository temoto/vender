package telenet

import (
	"log"
	"sync"
	"sync/atomic"
)

type ackmap struct {
	sync.Mutex
	m map[uint16]chan struct{}

	// atomic packed lastSeq uint16 + acks bitmap uint32
	v uint64
}

func (a *ackmap) Acked(seq uint16) (acked bool, ok bool) {
	last, bmap := a.read()
	if last == 0 {
		return false, false
	}
	if seq == last {
		return true, true
	}
	if seqGreater(last, seq+31) {
		return false, false
	}
	offset := last - seq
	return bmap&(1<<offset) != 0, true
}

func (a *ackmap) Cancel(seq uint16) {
	a.Lock()
	defer a.Unlock()
	a.cleanup(seq)
}

func (a *ackmap) Complete(seq uint16) {
	a.Lock()
	defer a.Unlock()
	a.complete(seq)
}

func (a *ackmap) Receive(sack uint16, bmap uint32) {
	if bmap == 0 {
		return
	}
	a.Lock()
	defer a.Unlock()
	for i := uint16(0); i < 32; i++ {
		if bmap&(1<<i) != 0 {
			a.complete(sack - i)
		}
	}
	a.cleanup(sack)
}

func (a *ackmap) Register(seq uint16) <-chan struct{} {
	a.Lock()
	defer a.Unlock()
	if ex := a.m[seq]; ex != nil {
		// code error but left unhandled would leak memory
		close(ex)
	}
	ch := make(chan struct{})
	a.m[seq] = ch
	return ch
}

func (a *ackmap) complete(seq uint16) {
	if ch := a.m[seq]; ch != nil {
		close(ch)
	}
}

func (a *ackmap) cleanup(before uint16) {
	cutoff := before - 32
	i := 0
	const n = 16
	// check up to n random elements, delete "expired"
	for seq := range a.m {
		expired := seqGreater(cutoff, seq)
		log.Printf("ackmap.cleanup before=%d cutoff=%d seq=%d expired=%t", before, cutoff, seq, expired)
		if expired {
			delete(a.m, seq)
		}
		if i++; i >= n {
			break
		}
	}
}

func (a *ackmap) read() (last uint16, bmap uint32) {
	v := atomic.LoadUint64(&a.v)
	bmap = uint32(v)
	last = uint16(v >> 32)
	return
}

func seqNext(addr *uint32) uint16 {
again:
	seq := uint16(atomic.AddUint32(addr, 1))
	if seq == 0 {
		goto again
	}
	return seq
}

func seqGreater(s1, s2 uint16) bool {
	return ((s1 > s2) && (s1-s2 <= 32768)) ||
		((s1 < s2) && (s2-s1 > 32768))
}
