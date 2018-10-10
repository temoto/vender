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
		name     string
		input    string
		expect   string
		expectFF string
	}
	cases := []Case{
		Case{"empty", "", "00", "ff0000"},
		Case{"contains-ff", "ff05", "ff0504", "ffff05ff0004"},
		Case{"validator-bill-type", "34012fffff", "34012fffff62", "34012fffffffffff0062"},
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
				t.Errorf("ffDance=false wire='%s' expected='%s'", wireRaw, c.expect)
			}
			wireFF := hex.EncodeToString(p.Wire(true))
			if wireFF != c.expectFF {
				t.Errorf("ffDance=true  wire='%s' expected='%s'", wireFF, c.expectFF)
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
