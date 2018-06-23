package mdb

import (
	"bufio"
	"bytes"
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
	do := func(t *testing.T, send, wexpects, recv, rexpects string, debug bool) {
		r := bytes.NewBuffer([]byte(recv))
		w := bytes.NewBuffer(nil)
		m := open(t, r, w)
		m.Debug = debug
		out := new(Packet)
		err := m.Tx(PacketFromString(send), out)
		if err != nil {
			t.Fatal(err)
		}
		wactual, wexpect := w.Bytes(), []byte(wexpects)
		if !bytes.Equal(wactual, wexpect) {
			t.Errorf("send actual='%x' expected='%x'", wactual, wexpect)
		}
		ractual, rexpect := out.Bytes(), []byte(rexpects)
		if !bytes.Equal(ractual, rexpect) {
			t.Errorf("recv actual='%x' expected='%x'", ractual, rexpect)
		}
	}
	t.Run("simple", func(t *testing.T) { do(t, "\x30", "\x30\x30", "\xff\x00\x00", "", false) })
	t.Run("complex", func(t *testing.T) {
		do(t, "\xca\x03", "\xca\x03\xcd\x00", "\xff\xff\x09\xff\x00\x08", "\xff\x09", true)
	})
}

func TestPacketFormat(t *testing.T) {
	p := new(Packet)
	// TODO: assert content equal
	t.Log(p.Format())
	p.l = 8
	t.Log(p.Format())
	p.l = 11
	t.Log(p.Format())
}

func BenchmarkTx1(b *testing.B) {
	m := open(b, nil, nil)
	response := new(Packet)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Tx(PacketNul1, response)
	}
}
