package telenet

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNextSeq(t *testing.T) {
	t.Parallel()

	var seq uint32
	atomic.StoreUint32(&seq, 0)
	go seqNext(&seq)
	assert.NotEqual(t, uint32(0), seqNext(&seq))

	atomic.StoreUint32(&seq, 1)
	go seqNext(&seq)
	assert.NotEqual(t, uint32(0), seqNext(&seq))

	atomic.StoreUint32(&seq, 0xffffffff)
	go seqNext(&seq)
	assert.NotEqual(t, uint32(0), seqNext(&seq))

	atomic.StoreUint32(&seq, 0xffffffff-1)
	go seqNext(&seq)
	assert.NotEqual(t, uint32(0), seqNext(&seq))
}
