package mdb

import (
	"bytes"
	"encoding/hex"
	"errors"
	"log"
	"strings"
	"testing"
)

const (
	PacketMaxLength = 40
)

var (
	ErrPacketOverflow = errors.New("mdb: operation larger than max packet size")
	ErrPacketReadonly = errors.New("mdb: packet is readonly")

	PacketEmpty = &Packet{readonly: true}
	PacketNul1  = &Packet{readonly: true, l: 1}
)

type Packet struct {
	b [PacketMaxLength]byte
	l int

	readonly bool
}

func PacketFromBytes(b []byte, readonly bool) (Packet, error) {
	p := Packet{}
	_, err := p.Write(b)
	if err != nil {
		return *PacketEmpty, err
	}
	p.readonly = readonly
	return p, nil
}
func MustPacketFromBytes(b []byte, readonly bool) Packet {
	p, err := PacketFromBytes(b, readonly)
	if err != nil {
		panic(err)
	}
	return p
}

func PacketFromHex(s string, readonly bool) (Packet, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return *PacketEmpty, err
	}
	return PacketFromBytes(b, readonly)
}
func MustPacketFromHex(s string, readonly bool) Packet {
	p, err := PacketFromHex(s, readonly)
	if err != nil {
		panic(err)
	}
	return p
}

func (self *Packet) Bytes() []byte {
	return self.b[:self.l]
}

func (self *Packet) Equal(p2 *Packet) bool {
	return self.l == p2.l && bytes.Equal(self.Bytes(), p2.Bytes())
}

func (self *Packet) write(p []byte) {
	self.l = copy(self.b[:], p)
}

func (self *Packet) Write(p []byte) (n int, err error) {
	if self.readonly {
		return 0, ErrPacketReadonly
	}
	pl := len(p)
	switch {
	case pl == 0:
		return 0, nil
	case pl > PacketMaxLength:
		return 0, ErrPacketOverflow
	}
	self.write(p)
	return self.l, nil
}

func (self *Packet) Len() int { return self.l }

func (self *Packet) Logf(format string) {
	log.Printf(format, self.Format())
}

func (self *Packet) Format() string {
	b := self.Bytes()
	h := hex.EncodeToString(b)
	hlen := len(h)
	ss := make([]string, (hlen/8)+1)
	for i := range ss {
		hi := (i + 1) * 8
		if hi > hlen {
			hi = hlen
		}
		ss[i] = h[i*8 : hi]
	}
	line := strings.Join(ss, " ")
	return line
}

func (self *Packet) Wire(ffDance bool) []byte {
	chk := byte(0)
	j := 0
	w := make([]byte, (self.l+2)*2)
	for _, b := range self.b[:self.l] {
		if ffDance && b == 0xff {
			w[j] = 0xff
			j++
		}
		w[j] = b
		j++
		chk += b
	}
	if ffDance {
		w[j] = 0xff
		w[j+1] = 0x00
		j += 2
	}
	w[j] = chk
	w = w[:j+1]
	return w
}

// Without checksum
func (self *Packet) TestHex(t testing.TB, expect string) {
	if _, err := hex.DecodeString(expect); err != nil {
		t.Fatalf("invalid expect=%s err=%s", expect, err)
	}
	actual := hex.EncodeToString(self.Bytes())
	if actual != expect {
		t.Fatalf("Packet=%s expected=%s", actual, expect)
	}
}
