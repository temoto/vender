package crc

import (
	"strings"
	"testing"
)

func makeCheck2(fun func(byte, byte) byte, tag string) func(t *testing.T, v1, v2, expect byte) {
	return func(t *testing.T, v1, v2, expect byte) {
		if fun(v1, v2) != expect {
			t.Errorf("%s(%02x, %02x) != %02x", tag, v1, v2, expect)
		}
	}
}

func makeCheckN(fun func(byte, []byte) byte, tag string) func(t *testing.T, v1 byte, vs []byte, expect byte) {
	return func(t *testing.T, v1 byte, vs []byte, expect byte) {
		if fun(v1, vs) != expect {
			t.Errorf("%s(%02x, "+strings.Repeat("%02x", len(vs))+") != %02x", tag, v1, vs, expect)
		}
	}
}

func TestReference(t *testing.T) {
	checkRef := makeCheck2(CRC8_p93_reference, "CRC8_p93_reference")
	checkRef(t, 0, 0x00, 0x00)
	checkRef(t, 0, 0x55, 0x86)
	checkRef(t, 0, 0xaa, 0x9f)
	checkRef(t, 0, 0xff, 0x19)
}

func TestLookup(t *testing.T) {
	checkNext := makeCheck2(CRC8_p93_next, "CRC8_p93_next")
	checkNext(t, 0, 0x00, 0x00)
	checkNext(t, 0, 0x55, 0x86)
	checkNext(t, 0, 0xaa, 0x9f)
	checkNext(t, 0, 0xff, 0x19)
	check2 := makeCheck2(CRC8_p93_2, "CRC8_p93_2")
	check2(t, 0x80, 0x00, 0x74)
	check2(t, 0xe0, 0x78, 0xc9)
	check2(t, 0x03, 0x01, 0xc8)
	checkN := makeCheckN(CRC8_p93_n, "CRC8_p93_n")
	checkN(t, 0, []byte{0x06, 0x00, 0xbe, 0xeb, 0xee}, 0x75)
	checkN(t, 0, []byte{0x04, 0x0f, 0x30}, 0xf7)
}
