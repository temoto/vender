package mega

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestNewCommand(t *testing.T) {
	t.Parallel()
	f := NewCommand(COMMAND_MDB_TRANSACTION_SIMPLE, 0xe1)
	expect := []byte{0x44, 0x02, 0x08, 0xe1, 0x1f}
	actual := f.Bytes()
	if !bytes.Equal(actual, expect) {
		t.Errorf("frame.Bytes() expected=%x actual=%x", expect, actual)
	}

	wire := bytes.Repeat([]byte{PROTOCOL_PAD_OK}, totalOverheads)
	copy(wire, actual)
	wire[len(actual)] = 0 // padding errcode
	f2 := new(Frame)
	err := f2.Parse(wire)
	if err != nil {
		t.Fatalf("Parse() back err=%v", err)
	}
}

func TestParse(t *testing.T) {
	t.Parallel()
	type Case struct {
		name      string
		input     string
		expect    string
		expectErr string
	}
	cases := []Case{
		{"input-under-length", "", "", "input length too small not valid"},
		{"empty-ok", "040000000101010101010101", "", ""},
		{"long", "04f1aaff000101010101010101", "", "frame=04f1aaff claims length=241 > input-overhead=1 not valid"},
		{"corrupted", "0401f4f2e7a9c2d9f5b4a62314", "", "frame=0401f4f2e7a9c2d9f5b4a62314 padding=b4a62314 not valid"},
		{"wrong-padding", "4403019cffffffffffffffffffffff", "01", "frame=4403019cffffffffffffffffffffff padding=ffffffff not valid"},
		{"invalid-header", "440100b800010101010101010101", "", "frame=440100b8 response=Response_t(0) not valid"},
		{"ok", "4401012b00010101010101010101", "01", ""},
		{"reset", "4409020200000101050302f90001010101", "020200000101050302", ""},
		{"status", "4409010204d40101050302d70001010101", "010204d40101050302", ""},
		// {"mdb-success-empty", "440701010a010081000101010101010101", "01010a0100", ""},
		// {"twi-listen", "440c03010306a3a308020031d300010101010101010101", "03010306a3a308020031", ""},
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input, err := hex.DecodeString(c.input)
			if err != nil {
				t.Fatalf("invalid input=%s err='%v'", c.input, err)
			}
			var f Frame
			errString := ""
			err = f.Parse(input)
			if err == nil {
				err = f.ParseFields()
			}
			if err != nil {
				errString = err.Error()
			} else {
				ps := fmt.Sprintf("%x", f.Payload())
				if ps != c.expect {
					t.Errorf("input=%s frame hex expected='%s' actual='%s'", c.input, c.expect, ps)
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
		Case{"status", "4409010204d40101050302d70001010101", "clock10u=12360us,firmware=0105,reset=+EXT"},
		// Case{"mdb-e0", "4409010a01000c015a0b00a600010101010101", "mdb_result=SUCCESS:00,mdb_duration=3460us,mdb_data="},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input, err := hex.DecodeString(c.input)
			if err != nil {
				t.Fatalf("invalid input=%s err='%v'", c.input, err)
			}

			f := new(Frame)
			err = f.Parse(input)
			if err == nil {
				err = f.ParseFields()
			}
			if err != nil {
				t.Fatalf("f.Parse input=%s err=%v", c.input, err)
			}
			fstr := f.Fields.String()
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
		data := [BUFFER_SIZE]byte{}
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
	type Case struct {
		name  string
		input string
	}
	cases := []Case{
		Case{"reset", "4409020200000101050302f90001010101"},
		Case{"status", "4409010204d40101050302d70001010101"},
		Case{"mdb-long", "441c0102141b1001001208a2111006000b0100010a06d807362800000701080001010101"},
	}

	mkBench := func(input []byte) func(b *testing.B) {
		return func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			f := Frame{}
			b.ResetTimer()
			for i := 1; i <= b.N; i++ {
				err := f.Parse(input)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	}

	for _, c := range cases {
		c := c
		inputBytes, err := hex.DecodeString(c.input)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(c.name, mkBench(inputBytes))
	}
}
