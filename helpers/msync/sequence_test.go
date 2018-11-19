package msync

import (
	"io/ioutil"
	"log"
	"testing"
)

func testAction(w *MultiWait, args interface{}) (err error) {
	return
}

func BenchmarkSequenceLen1(b *testing.B) {
	log.SetFlags(0)
	log.SetOutput(ioutil.Discard)
	s := NewSequence("len1")
	s.Append(NewAction("test", testAction))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Start()
		s.Wait()
	}
}
