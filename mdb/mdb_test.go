package mdb

import (
	"io/ioutil"
	"log"
	"testing"
)

func BenchmarkTx1(b *testing.B) {
	log.SetFlags(0)
	log.SetOutput(ioutil.Discard)
	m := new(MDB)
	m.skipIO = true
	err := m.Open("/dev/null", 9600, 1)
	if err != nil {
		b.Fatal(err)
	}
	bout := [2]byte{0, 0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Tx(bout[:1], nil)
	}
}
