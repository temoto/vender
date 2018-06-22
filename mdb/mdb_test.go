package mdb

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"log"
	"testing"
)

type Fataler interface {
	Fatal(...interface{})
}

func open(t Fataler, r, w *bytes.Buffer) *MDB {
	m := new(MDB)
	m.skip_ioctl = true
	err := m.Open("/dev/null", 9600, 1)
	if err != nil {
		t.Fatal()
	}
	if r == nil {
		r = bytes.NewBuffer(nil)
	}
	if w == nil {
		w = bytes.NewBuffer(nil)
	}
	m.r = bufio.NewReader(r)
	m.w = w
	return m
}

func TestTx1(t *testing.T) {
	do := func(t *testing.T, send, written, read string) {
		r := bytes.NewBuffer([]byte(read))
		w := bytes.NewBuffer(nil)
		m := open(t, r, w)
		out := make([]byte, 0, MaxPacketLength)
		err := m.Tx([]byte(send), out)
		if err != nil {
			t.Fatal(err)
		}
		wactual, wexpect := w.Bytes(), []byte(written)
		if !bytes.Equal(wactual, wexpect) {
			t.Errorf("send actual='%x' expected='%x'", wactual, wexpect)
		}
		ractual, rexpect := out, r.Bytes()
		if !bytes.Equal(ractual, rexpect) {
			t.Errorf("recv actual='%x' expected='%x'", ractual, rexpect)
		}
	}
	t.Run("simple", func(t *testing.T) { do(t, "\x30", "\x30\x30", "\xff\x00\x00") })
	t.Run("complex", func(t *testing.T) { do(t, "\xca\x03", "\xca\x03\xcd\x00\x00", "\xff\xff\x09\xff\x00\x08") })
}

func BenchmarkTx1(b *testing.B) {
	log.SetFlags(0)
	log.SetOutput(ioutil.Discard)
	m := open(b, nil, nil)
	bout := [2]byte{0, 0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Tx(bout[:1], nil)
	}
}
