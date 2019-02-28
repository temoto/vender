package mega

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Parallel()
	type Case struct {
		name      string
		input     string
		expect    string
		expectErr string
	}
	cases := []Case{
		{"empty", "", "", "packet empty not valid"},
		{"long", "f1", "", "packet=f1 claims length=241 > max=70 not valid"},
		{"under-length", "04", "", "packet=04 claims length=4 > input=1 not valid"},
		{"short", "01", "", "packet=01 claims length=1 < min=4 not valid"},
		{"corrupted", "040000ff", "", "packet=040000ff crc=ff actual=86 not valid"},
		{"invalid-header", "04000086", "", "packet=04000086 header=00 not valid"},
		{"ok", "04000115", "00:01", ""},
		{"ok-and-garbage", "04d0019cffffff", "d0:01", ""},
		{"mdb-success-empty", "088501010a010081", "85:01010a0100", ""},
		{"with-fields", "12170101030504060a1507000a01ff0b00ef", "17:0101030504060a1507000a01ff0b00", ""},
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input, err := hex.DecodeString(c.input)
			if err != nil {
				t.Fatalf("invalid input=%s err='%v'", c.input, err)
			}
			var p Packet
			errString := ""
			err = p.Parse(input)
			if err != nil {
				errString = err.Error()
			} else {
				ps := fmt.Sprintf("%02x:%s", p.Id, p.SimpleHex())
				if ps != c.expect {
					t.Errorf("input=%s packets expected='%s' actual='%s'", c.input, c.expect, ps)
				}
			}
			if errString != c.expectErr {
				t.Errorf("input=%s error expected='%v' actual='%v'", c.input, c.expectErr, err)
			}
		})
	}
}

func TestParseFields(t *testing.T) {
	t.Parallel()
	type Case struct {
		name   string
		input  string
		expect string
	}
	cases := []Case{
		Case{"mdb-e0", "0c3e010a01000c015a0b004f", "mdb_result=SUCCESS:00,mdb_duration=3460us,mdb_data="},
		Case{"status", "1258010103020103050607000900063a799e", "protocol=3,firmware=0103,reset=+EXT+BO,twi_length=0,mdb_length=0,clock10u=149690us"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input, err := hex.DecodeString(c.input)
			if err != nil {
				t.Fatalf("invalid input=%s err='%v'", c.input, err)
			}

			p := Packet{}
			err = p.Parse(input)
			if err != nil {
				t.Fatalf("p.Parse input=%s err=%v", c.input, err)
			}
			fstr := p.Fields.String()
			if fstr != c.expect {
				t.Errorf("fields expected='%s' actual='%s'", c.expect, fstr)
			}
		})
	}
}

func TestFieldCover(t *testing.T) {
	t.Parallel()
	fs := Fields{}
	invalidTagPrefix := "!ERROR:invalid-tag"
	if !strings.HasPrefix(fs.FieldString(FIELD_INVALID), invalidTagPrefix) {
		t.Fatalf("test code error: invalid tag prefix")
	}

	for f8 := uint8(1); f8 != 0; f8++ {
		f := Field_t(f8)
		fstr := f.String()
		should := !strings.HasPrefix(fstr, "Field_t(")
		hasString := !strings.HasPrefix(fs.FieldString(f), invalidTagPrefix)
		data := [RESPONSE_MAX_LENGTH]byte{}
		data[0] = f8
		hasParse := false
		if pf, _ := fs.parseNext(data[:]); pf != FIELD_INVALID {
			hasParse = true
		}
		if should && !hasString {
			t.Errorf("tag=%02x name=%s defined in firmware but no String() in client", f8, fstr)
		}
		if should && !hasParse {
			t.Errorf("tag=%02x name=%s defined in firmware but no parser in client", f8, fstr)
		}
		if !should && (hasParse || hasString) {
			t.Errorf("tag=%02x not defined in firmware (anymore?)", f8)
		}
	}
}

func BenchmarkParse(b *testing.B) {
	packet1 := NewPacket(1, byte(RESPONSE_OK),
		byte(FIELD_PROTOCOL), 3,
		byte(FIELD_FIRMWARE_VERSION), 0x01, 0x02,
		byte(FIELD_MDB_DATA), 2, 0x0d, 0x00,
	)
	packet2 := NewPacket(2, byte(RESPONSE_OK), byte(FIELD_MDB_RESULT), byte(MDB_RESULT_SUCCESS), 0)
	packet3 := NewPacket(0, byte(RESPONSE_RESET), byte(FIELD_PROTOCOL), 3)

	mkBench := func(input []byte) func(b *testing.B) {
		return func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			p := Packet{}
			b.ResetTimer()
			for i := 1; i <= b.N; i++ {
				err := p.Parse(input)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	}

	for i, p := range []Packet{packet1, packet2, packet3} {
		p := p
		b.Run(fmt.Sprint(i), mkBench(p.Bytes()))
	}
}
