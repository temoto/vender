package mega

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestParseResponse(t *testing.T) {
	t.Parallel()
	type Case struct {
		name      string
		input     string
		expect    string
		expectErr string
	}
	cases := []Case{
		{"response-empty", "", "", "response empty not valid"},
		{"response-empty-valid-length", "00", "", ""},
		{"response-length-max", "f1", "", "response=f1 claims length=241 > max=80 not valid"},
		{"response-under-length", "01", "", "response=01 claims length=1 > input=0 not valid"},
		{"packet-short", "0101", "", "packet=01 claims length=1 < min=4 not valid"},
		{"packet-long", "01ff", "", "packet=ff claims length=255 > max=80 not valid"},
		{"packet-under-length", "0104", "", "packet=04 claims length=4 > input=1 not valid"},
		{"packet-corrupted", "04040000ff", "", "packet=040000ff crc=ff actual=86 not valid"},
		{"packet-invalid-header", "0404000086", "", "packet=04000086 header=00 not valid"},
		{"ok", "0404000115", "00:01", ""},
		{"ok-and-garbage", "0404d0019cffffff", "d0:01", ""},
		{"ok-and-short", "060485018c04ff", "85:01", "packet=04ff claims length=4 > input=2 not valid"},
		{"debug-beebee", "07070004beebeefe", "00:04beebee", ""},
		{"mdb-success", "04043e0821", "3e:08", ""},
		{"mdb-success-and-twi", "09041508cd0500063077", "15:08,00:0630", ""},
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			// t.Parallel()
			input, err := hex.DecodeString(c.input)
			if err != nil {
				t.Fatalf("invalid input=%s err='%v'", c.input, err)
			}
			var ps []string
			err = ParseResponse(input, func(p Packet) {
				ps = append(ps, fmt.Sprintf("%02x:%s", p.Id, p.SimpleHex()))
			})
			errString := ""
			if err != nil {
				errString = err.Error()
			}
			if errString != c.expectErr {
				t.Errorf("input=%s expected err='%v' actual='%v'", c.input, c.expectErr, err)
			}
			psjoined := strings.Join(ps, ",")
			if psjoined != c.expect {
				t.Errorf("input=%s expected packets='%s' actual='%s'", c.input, c.expect, psjoined)
			}
		})
	}
}

func TestParseFields(t *testing.T) {
	t.Parallel()
	packet := NewPacket(0, byte(RESPONSE_JUST_RESET),
		byte(FIELD_PROTOCOL), PROTOCOL_VERSION,
		byte(FIELD_FIRMWARE_VERSION), 0x01, 0x02,
		byte(FIELD_BEEBEE), 0xbe, 0xeb, 0xee,
	)
	result := ParseFields(packet.Data())
	expect := "protocol=2,firmware=0102,beebee-mark"
	if result != expect {
		t.Errorf("expected fields=%s actual=%s", expect, result)
	}
}

func BenchmarkParseResponse(b *testing.B) {
	input := make([]byte, 1, RESPONSE_MAX_LENGTH)
	packet1 := NewPacket(0, byte(RESPONSE_JUST_RESET),
		byte(FIELD_PROTOCOL), PROTOCOL_VERSION,
		byte(FIELD_FIRMWARE_VERSION), 0x01, 0x02,
		byte(FIELD_BEEBEE), 0xbe, 0xeb, 0xee,
	)
	packet2 := NewPacket(2, byte(RESPONSE_MDB_SUCCESS))
	packet3 := NewPacket(0, byte(RESPONSE_TWI), 0x30)
	input = append(input, packet1.Bytes()...)
	input = append(input, packet2.Bytes()...)
	input = append(input, packet3.Bytes()...)
	input[0] = byte(len(input) - 1)
	drop := Packet{}

	mkBench := func(fun func(Packet)) func(b *testing.B) {
		return func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 1; i <= b.N; i++ {
				b.SetBytes(int64(len(input)))
				err := ParseResponse(input, fun)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	}

	b.Run("Bytes", mkBench(func(p Packet) { _ = p.Bytes() }))
	b.Run("String", mkBench(func(p Packet) { _ = p.String() }))
	b.Run("ParseFields", mkBench(func(p Packet) { _ = ParseFields(p.Data()) }))

	_ = drop
}
