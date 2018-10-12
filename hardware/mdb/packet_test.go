package mdb

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"strings"
	"testing"
	"time"
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
		{"empty", "", "00", "ff0000"},
		{"contains-ff", "ff05", "ff0504", "ffff05ff0004"},
		{"validator-bill-type", "34012fffff", "34012fffff62", "34012fffffffffff0062"},
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
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

func TestPacketFormat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		p      *Packet
		expect string
	}{
		{"empty", new(Packet), ""},
		{"short", &Packet{l: 3}, "000000"},
		{"long", PacketFromString("0q9w8e7r6t5"), "30713977 38653772 367435"},
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := c.p.Format()
			if f != c.expect {
				t.Fatalf("Packet=%v format=%s expected=%s", c.p, f, c.expect)
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
		{"hex", func() *Packet { return PacketFromHex("invalid hex") }},
		{"bytes", func() *Packet { return PacketFromBytes(bytes.Repeat([]byte{'.'}, 41)) }},
		{"string", func() *Packet { return PacketFromString(strings.Repeat("long", 20)) }},
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := c.fun()
			if p != nil {
				t.Fatalf("expected nil parsed='%x'", p.Wire(false))
			}
		})
	}
}
