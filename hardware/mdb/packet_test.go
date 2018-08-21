package mdb

import (
	"encoding/hex"
	"testing"
)

func TestPacketFromHexToWire(t *testing.T) {
	t.Parallel()
	type Case struct {
		name   string
		input  string
		expect string
	}
	cases := []Case{
		Case{"empty", "", "00"},
		Case{"validator-bill-type", "34012fffff", "34012fffff62"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := hex.DecodeString(c.expect); err != nil {
				t.Fatalf("invalid expect=%s err=%s", c.expect, err)
			}
			p := PacketFromHex(c.input)
			if p == nil {
				t.Fatalf("input=%s parsed nil", c.input)
			}
			wireRaw := hex.EncodeToString(p.Wire(false))
			if wireRaw != c.expect {
				t.Errorf("wire='%s' expected='%s'", wireRaw, c.expect)
			}
		})
	}
}
