package mdb

import (
	"bytes"
	"encoding/hex"
	"io"
	"testing"

	"github.com/juju/errors"
	"github.com/temoto/vender/helpers"
)

func testMdberStrings(t testing.TB, r io.Reader, w io.Writer) Mdber {
	m, err := NewMDB(NewNullUart(r, w), "", 0)
	if err != nil {
		t.Fatal(errors.ErrorStack(err))
	}
	m.SetLog(t.Logf)
	return m
}

func checkTx(t testing.TB, send, recv *Packet, wexpects, rexpects string) {
	helpers.LogToTest(t)
	t.Logf("send=%s wexp=%x recv=%s rexp=%x", send.Format(), wexpects, recv.Format(), rexpects)
	r := bytes.NewReader(recv.Bytes())
	w := bytes.NewBuffer(nil)
	m := testMdberStrings(t, r, w)
	out := new(Packet)
	err := m.Tx(send, out)
	if err != nil {
		t.Fatal(errors.ErrorStack(err))
	}
	wactual := w.Bytes()
	wexpect, err := hex.DecodeString(wexpects)
	if err != nil {
		panic(errors.Errorf("code error invalid hex wexpects=%s err=%v", wexpects, err))
	}
	if !bytes.Equal(wactual, wexpect) {
		t.Errorf("send actual='%x' expected='%x'", wactual, wexpect)
	}
	ractual := out.Bytes()
	rexpect, err := hex.DecodeString(rexpects)
	if err != nil {
		panic(errors.Errorf("code error invalid hex rexpects=%s err=%v", rexpects, err))
	}
	if !bytes.Equal(ractual, rexpect) {
		t.Errorf("recv actual='%x' expected='%x'", ractual, rexpect)
	}
}

// Mdber.Tx() only wraps Uarter.Tx() in Packet-s
func TestTx1(t *testing.T) {
	cases := []struct {
		name    string
		send    string
		wexpect string
		recv    string
		rexpect string
	}{
		{"simple", "30", "3030", "ff0000", ""},
		{"complex", "ca03", "ca03cd00", "ffff09ff0008", "ff09"},
	}
	helpers.RandUnix().Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkTx(t, PacketFromHex(c.send), PacketFromHex(c.recv), c.wexpect, c.rexpect)
		})
	}
}
