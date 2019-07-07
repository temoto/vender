package cacheval

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInt32Valid(t *testing.T) {
	t.Parallel()

	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	const valid = 100 * time.Millisecond

	cv := Int32{}
	cv.Init(valid)

	assert.Equal(t, int32(0), cv.Get())
	v, ok := cv.GetFresh()
	assert.Equal(t, int32(0), v)
	assert.Equal(t, false, ok)

	expect := int32(rand.Uint32())
	cv.Set(expect)
	v, ok = cv.GetFresh()
	assert.Equal(t, expect, v)
	assert.Equal(t, true, ok)

	time.Sleep(valid)
	v = cv.GetOrUpdate(func() { cv.Set(expect + 1) })
	assert.Equal(t, expect+1, v)
	assert.Equal(t, expect+1, cv.Get())
}

func TestInt32Stress(t *testing.T) {
	const valid = 10 * time.Millisecond
	const delay = 1 * time.Millisecond
	const concurrency = 50
	const N = 500

	cv := Int32{}
	cv.Init(valid)

	wg := sync.WaitGroup{}
	wg.Add(concurrency)
	passive := func() {
		max := int32(0)
		for j := 1; j <= N; j++ {
			v, ok := cv.GetFresh()
			if v > max {
				max = v
			} else if ok && v < max {
				t.Error("unexpected decrease")
			}
			time.Sleep(delay)
		}
		wg.Done()
	}
	for i := 1; i <= concurrency; i++ {
		go passive()
	}
	for j := 1; j <= N; j++ {
		prev := cv.Get()
		cv.GetOrUpdate(func() { cv.Set(prev + 1) })
		time.Sleep(delay)
	}
	wg.Wait()
}
