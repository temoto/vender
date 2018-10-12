package mdb

import (
	"bytes"
	"io"
	"testing"

	"github.com/temoto/vender/helpers"
)

func open(t helpers.Fataler, r io.Reader, w io.Writer) *mdb {
	m, err := NewMDB(NewNullUart(r, w), "", 9600)
	if err != nil {
		t.Fatal(err)
	}
	m.SetLog(t.Logf)
	return m
}

func TestTx1(t *testing.T) {
	do := func(t *testing.T, send, wexpects, recv, rexpects string, debug bool) {
		r := bytes.NewReader([]byte(recv))
		w := bytes.NewBuffer(nil)
		m := open(t, r, w)
		m.SetDebug(debug)
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
	m := open(b, bytes.NewBufferString(""), bytes.NewBuffer(nil))
	m.SetLog(helpers.Discardf)
	response := new(Packet)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Tx(PacketNul1, response)
	}
}
