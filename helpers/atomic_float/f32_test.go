package atomic_float

import (
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestF32Stress(t *testing.T) {
	const concurrency = 150
	const N = 5000
	const step = 0.001

	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	var f F32
	initial := (rand.Float32() - 0.5) * (1 << 16)
	f.Store(initial)

	wg := sync.WaitGroup{}
	wg.Add(concurrency)
	fun := func() {
		max := float32(-math.MaxFloat32)
		for j := 1; j <= N; j++ {
			v := f.Load()
			if v > max {
				max = v
			} else if v < max {
				t.Error("unexpected decrease")
			}
			f.Add(step)
		}
		wg.Done()
	}
	for i := 1; i <= concurrency; i++ {
		go fun()
	}
	wg.Wait()
	expect := initial
	for i := 1; i <= concurrency*N; i++ {
		expect += step // match accumulated precision error
	}
	delta := float32(float64(N*concurrency) * float64(step))
	final := f.Load()
	assert.Equal(t, expect, final, "initial=%f delta=%f final=%f", initial, delta, final)
}
