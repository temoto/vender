package mdb

import (
	"bytes"
	"encoding/hex"
	"strings"
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

func TestInvalidPacketFrom(t *testing.T) {
	t.Parallel()
	type Case struct {
		name string
		fun  func() *Packet
	}
	cases := []Case{
		Case{"hex", func() *Packet { return PacketFromHex("invalid hex") }},
		Case{"bytes", func() *Packet { return PacketFromBytes(bytes.Repeat([]byte{'.'}, 41)) }},
		Case{"string", func() *Packet { return PacketFromString(strings.Repeat("long", 20)) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := c.fun()
			if p != nil {
				t.Fatalf("expected nil parsed='%x'", p.Wire(false))
			}
		})
	}
}
